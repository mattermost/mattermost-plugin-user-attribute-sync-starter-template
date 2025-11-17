package sync

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// formatStringValue converts text or date values to the JSON format required by PropertyService.
func formatStringValue(value string) (json.RawMessage, error) {
	marshaled, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal string value: %w", err)
	}

	return json.RawMessage(marshaled), nil
}

// formatMultiselectValue converts multiselect option names to option IDs in JSON format.
// Multiselect fields store arrays of option IDs, not human-readable names.
// Works with any multiselect field that has options in the cache.
func formatMultiselectValue(fieldName string, values []string, cache *FieldIDCache) (json.RawMessage, error) {
	// Translate option names to IDs
	optionIDs := make([]string, 0, len(values))
	for _, optionName := range values {
		optionID := cache.GetOptionID(optionName)
		if optionID == "" {
			return nil, fmt.Errorf("unknown option %q for field %s", optionName, fieldName)
		}
		optionIDs = append(optionIDs, optionID)
	}

	marshaled, err := json.Marshal(optionIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal multiselect value: %w", err)
	}

	return json.RawMessage(marshaled), nil
}

// buildPropertyValue creates a single PropertyValue for a user attribute.
// Returns nil, nil for fields that should be skipped (like "email").
// Returns nil, error for fields that fail validation or formatting.
func buildPropertyValue(
	api *pluginapi.Client,
	user *model.User,
	groupID string,
	fieldName string,
	fieldValue interface{},
	cache *FieldIDCache,
) (*model.PropertyValue, error) {
	// Use Email to map external attributes to users, not synced as an attribute.
	// An extender of this plugin template could choose some other way to identify users.
	if fieldName == "email" {
		return nil, nil
	}

	fieldID := cache.GetFieldID(fieldName)
	if fieldID == "" {
		api.Log.Warn("Unknown field name, skipping",
			"field_name", fieldName,
			"user_email", user.Email)
		return nil, nil
	}

	// Format value based on type
	var formattedValue json.RawMessage
	var formatErr error

	switch v := fieldValue.(type) {
	case []interface{}:
		// Multiselect - convert to string array
		stringValues := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				stringValues = append(stringValues, str)
			}
		}
		formattedValue, formatErr = formatMultiselectValue(fieldName, stringValues, cache)

	case []string:
		// Multiselect - already string array
		formattedValue, formatErr = formatMultiselectValue(fieldName, v, cache)

	case string:
		// Text or date
		formattedValue, formatErr = formatStringValue(v)

	default:
		api.Log.Warn("Unsupported field value type, skipping field",
			"field_name", fieldName,
			"user_email", user.Email,
			"value_type", fmt.Sprintf("%T", fieldValue))
		return nil, nil
	}

	if formatErr != nil {
		api.Log.Warn("Failed to format field value, skipping field",
			"field_name", fieldName,
			"user_email", user.Email,
			"error", formatErr.Error())
		return nil, nil
	}

	propertyValue := &model.PropertyValue{
		GroupID:    groupID,
		TargetType: "user",
		TargetID:   user.Id,
		FieldID:    fieldID,
		Value:      formattedValue,
	}

	return propertyValue, nil
}

// buildPropertyValues creates PropertyValue objects for a single user's attributes.
func buildPropertyValues(api *pluginapi.Client, user *model.User, groupID string, userAttrs map[string]interface{}, cache *FieldIDCache) ([]*model.PropertyValue, error) {
	values := make([]*model.PropertyValue, 0, len(userAttrs))

	for fieldName, fieldValue := range userAttrs {
		propertyValue, err := buildPropertyValue(api, user, groupID, fieldName, fieldValue, cache)
		if err != nil {
			return nil, err
		}
		if propertyValue != nil {
			values = append(values, propertyValue)
		}
	}

	return values, nil
}

// SyncUsers writes attribute values from external data into Mattermost CPAs for all users.
//
//nolint:revive
func SyncUsers(api *pluginapi.Client, groupID string, users []map[string]interface{}, cache *FieldIDCache) error {
	for _, userAttrs := range users {
		email, ok := userAttrs["email"].(string)
		if !ok || email == "" {
			api.Log.Warn("User object missing email field, skipping")
			continue
		}

		// Find Mattermost user by email
		user, err := api.User.GetByEmail(email)
		if err != nil {
			api.Log.Warn("User not found by email, skipping",
				"email", email,
				"error", err.Error())
			continue
		}

		values, err := buildPropertyValues(api, user, groupID, userAttrs, cache)
		if err != nil {
			api.Log.Error("Failed to build property values, skipping user",
				"user_email", email,
				"error", err.Error())
			continue
		}

		if len(values) == 0 {
			api.Log.Debug("No property values to sync for user", "email", email)
			continue
		}

		// Write all values for this user to Mattermost
		_, err = api.Property.UpsertPropertyValues(values)
		if err != nil {
			api.Log.Error("Failed to upsert property values, skipping user",
				"user_email", email,
				"value_count", len(values),
				"error", err.Error())
			continue
		}

		api.Log.Debug("Successfully synced user attributes",
			"email", email,
			"attribute_count", len(values))
	}

	return nil
}
