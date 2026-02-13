package sync

import (
	"encoding/json"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

// FieldIDCache stores mappings from external field/option names to Mattermost-generated IDs.
// These IDs are dynamically loaded during plugin activation by creating fields and looking up their IDs.
type FieldIDCache struct {
	// Maps external field names (e.g., "job_title") to Mattermost field IDs
	FieldNameToID map[string]string
	// Maps multiselect option names (e.g., "Apples") to Mattermost option IDs for all multiselect fields
	OptionNameToID map[string]string
}

// GetFieldID translates an external field name to its Mattermost field ID.
func (c *FieldIDCache) GetFieldID(fieldName string) string {
	return c.FieldNameToID[fieldName]
}

// GetOptionID translates a multiselect option name to its Mattermost option ID.
func (c *FieldIDCache) GetOptionID(optionName string) string {
	return c.OptionNameToID[optionName]
}

// fieldDefinition defines a Custom Profile Attribute field schema.
type fieldDefinition struct {
	Name         string                  // Display name shown in UI
	ExternalName string                  // Name used in external data source
	Type         model.PropertyFieldType // Field type (text, date, multiselect, etc.)
	OptionNames  []string                // Option names for multiselect fields
	AccessMode   string                  // Read permissions for field
}

// fieldDefinitions contains all Custom Profile Attribute fields this plugin creates.
// Custom Profile Attributes (CPAs) are user metadata fields that appear in user profiles.
// This plugin ensures these fields exist on startup and syncs external data into them.
var fieldDefinitions = []fieldDefinition{
	{
		Name:         "Job Title",
		ExternalName: "job_title",
		Type:         model.PropertyFieldTypeText,
		AccessMode:   model.PropertyAccessModePublic,
	},
	{
		Name:         "Programs",
		ExternalName: "programs",
		Type:         model.PropertyFieldTypeMultiselect,
		OptionNames:  []string{"Apples", "Oranges", "Lemons", "Grapes"},
		AccessMode:   model.PropertyAccessModeSharedOnly,
	},
	{
		Name:         "Start Date",
		ExternalName: "start_date",
		Type:         model.PropertyFieldTypeDate,
		AccessMode:   model.PropertyAccessModeSourceOnly,
	},
}

// updateField updates an existing CPA field to match the definition.
// Returns the updated field.
func updateField(
	client *pluginapi.Client,
	groupID string,
	existingField *model.PropertyField,
	def fieldDefinition,
) (*model.PropertyField, error) {
	client.Log.Info("Field exists, updating to match definition",
		"field_id", existingField.ID,
		"name", def.Name)

	existingField.Type = def.Type
	existingField.Attrs[model.CustomProfileAttributesPropertyAttrsVisibility] = model.CustomProfileAttributesVisibilityAlways
	existingField.Attrs[model.PropertyAttrsProtected] = true

	if def.Type == model.PropertyFieldTypeMultiselect {
		// Build options array with name only - Mattermost will generate IDs
		options := make([]interface{}, len(def.OptionNames))
		for i, optionName := range def.OptionNames {
			options[i] = map[string]interface{}{
				"name": optionName,
			}
		}
		existingField.Attrs[model.PropertyFieldAttributeOptions] = options
	}

	updatedField, err := client.Property.UpdatePropertyField(groupID, existingField)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update existing field %s", def.Name)
	}

	client.Log.Info("Updated field successfully", "field_id", updatedField.ID, "name", def.Name)
	return updatedField, nil
}

// createField creates a new CPA field from the definition.
// Returns the newly created field.
func createField(
	client *pluginapi.Client,
	groupID string,
	def fieldDefinition,
) (*model.PropertyField, error) {
	client.Log.Info("Field does not exist, creating", "name", def.Name)

	field := &model.PropertyField{
		// ID left empty - Mattermost will auto-generate
		GroupID: groupID,
		Name:    def.Name,
		Type:    def.Type,
		Attrs: model.StringInterface{
			// Always visible in UI so users can see the values
			model.CustomProfileAttributesPropertyAttrsVisibility: model.CustomProfileAttributesVisibilityAlways,
			// Protected to prevent admins from modifying field structure
			model.PropertyAttrsProtected: true,
			// Set configured read access mode
			model.PropertyAttrsAccessMode: def.AccessMode,
		},
	}

	// Multiselect fields need their options defined
	if def.Type == model.PropertyFieldTypeMultiselect {
		// Build options array with name only - Mattermost will generate IDs
		options := make([]interface{}, len(def.OptionNames))
		for i, optionName := range def.OptionNames {
			options[i] = map[string]interface{}{
				"name": optionName,
			}
		}
		field.Attrs[model.PropertyFieldAttributeOptions] = options
	}

	createdField, err := client.Property.CreatePropertyField(field)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create field %s", def.Name)
	}

	client.Log.Info("Created field successfully", "field_id", createdField.ID, "name", def.Name)
	return createdField, nil
}

// syncSingleField ensures a single CPA field exists and matches the definition.
// Updates the cache with field and option IDs. Returns the field ID or error.
func syncSingleField(
	client *pluginapi.Client,
	groupID string,
	def fieldDefinition,
	cache *FieldIDCache,
) (string, error) {
	// Try to get existing field
	existingField, err := client.Property.GetPropertyFieldByName(groupID, "", def.Name)

	var field *model.PropertyField
	if err == nil && existingField != nil {
		// Field exists - update it
		field, err = updateField(client, groupID, existingField, def)
		if err != nil {
			return "", err
		}
	} else {
		// Field doesn't exist - create it
		field, err = createField(client, groupID, def)
		if err != nil {
			return "", err
		}
	}

	// Store the field name to ID mapping
	cache.FieldNameToID[def.ExternalName] = field.ID

	// For multiselect fields, extract option IDs
	if def.Type == model.PropertyFieldTypeMultiselect && len(def.OptionNames) > 0 {
		if err := extractOptionIDs(client, field, def, cache); err != nil {
			client.Log.Error("Failed to extract option IDs",
				"name", def.Name,
				"field_id", field.ID,
				"error", err.Error())
			// Don't fail the entire sync, just log the error
		}
	}

	return field.ID, nil
}

// extractOptionIDs extracts option IDs from a multiselect field into the cache.
// Avoids adding duplicate options with the same name.
func extractOptionIDs(
	client *pluginapi.Client,
	field *model.PropertyField,
	def fieldDefinition,
	cache *FieldIDCache,
) error {
	// Extract option IDs from the field attributes
	optionsRaw, ok := field.Attrs[model.PropertyFieldAttributeOptions]
	if !ok {
		return errors.New("field has no options attribute")
	}

	// Convert options to JSON and back to extract IDs
	optionsJSON, err := json.Marshal(optionsRaw)
	if err != nil {
		return errors.Wrap(err, "failed to marshal options")
	}

	var options []map[string]interface{}
	if err := json.Unmarshal(optionsJSON, &options); err != nil {
		return errors.Wrap(err, "failed to unmarshal options")
	}

	// Build option name to ID mapping for all multiselect fields
	for _, opt := range options {
		name, nameOk := opt["name"].(string)
		id, idOk := opt["id"].(string)
		if !nameOk || !idOk {
			continue
		}

		// Avoid duplicate option names - only add if not already in cache
		if _, exists := cache.OptionNameToID[name]; !exists {
			cache.OptionNameToID[name] = id
		}
	}

	client.Log.Debug("Extracted option IDs",
		"field_name", def.Name,
		"option_count", len(options))

	return nil
}

// SyncFields ensures all CPA fields exist and match the definitions.
// Returns a FieldIDCache containing mappings from external names to Mattermost-generated IDs.
//
//nolint:revive
func SyncFields(client *pluginapi.Client, groupID string) (*FieldIDCache, error) {
	client.Log.Info("Syncing field definitions", "field_count", len(fieldDefinitions))

	cache := &FieldIDCache{
		FieldNameToID:  make(map[string]string),
		OptionNameToID: make(map[string]string),
	}

	var failedFields []string

	for _, def := range fieldDefinitions {
		_, err := syncSingleField(client, groupID, def, cache)
		if err != nil {
			client.Log.Error("Failed to sync field",
				"name", def.Name,
				"error", err.Error())
			failedFields = append(failedFields, def.Name)
			// Continue with next field for graceful degradation
			continue
		}
	}

	if len(failedFields) > 0 {
		client.Log.Warn("Some fields failed to sync",
			"failed_count", len(failedFields),
			"failed_fields", failedFields)
		// Return partial cache even on failures
	}

	client.Log.Info("Field sync completed",
		"total", len(fieldDefinitions),
		"failed", len(failedFields),
		"fields_cached", len(cache.FieldNameToID),
		"options_cached", len(cache.OptionNameToID))

	return cache, nil
}
