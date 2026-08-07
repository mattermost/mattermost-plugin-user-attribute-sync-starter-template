package sync

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

// FieldIDCache stores mappings from external field/option names to Mattermost-generated IDs.
// These IDs are dynamically loaded during plugin activation by creating fields and looking up their IDs.
type FieldIDCache struct {
	// Maps external field names (e.g., "job_title") to Mattermost field IDs
	FieldNameToID map[string]string
	// Maps option names (e.g., "Apples") to Mattermost option IDs for all select/multiselect/rank fields
	OptionNameToID map[string]string
}

// GetFieldID translates an external field name to its Mattermost field ID.
func (c *FieldIDCache) GetFieldID(fieldName string) string {
	return c.FieldNameToID[fieldName]
}

// GetOptionID translates a select/multiselect/rank option name to its Mattermost option ID.
func (c *FieldIDCache) GetOptionID(optionName string) string {
	return c.OptionNameToID[optionName]
}

// fieldDefinition defines a user attribute field schema.
type fieldDefinition struct {
	// Name is the canonical field identifier. It must match ^[A-Za-z_][A-Za-z0-9_]*$
	// because Mattermost references the name from ABAC policy expressions as
	// user.attributes.<name> (a CEL identifier), so spaces and punctuation are
	// rejected. We also use this name as the lookup key when matching attributes
	// from the external data source — the JSON file's keys must match these names.
	Name string

	// DisplayName is the human-readable label shown in user-facing UI. Free-form
	// text; no character restrictions.
	DisplayName string

	Type    model.PropertyFieldType                     // Field type (text, date, multiselect, etc.)
	Options []model.CustomProfileAttributesSelectOption // Options for select, multiselect, and rank fields
	// AccessMode controls who can read this field's values. Three modes:
	//   - Public (empty string): Everyone can read all field options and values
	//   - SourceOnly: Only this plugin can read values; others see empty options and no values
	//   - SharedOnly: Users only see field options and values they share with the target user
	//                 (Only valid for select/multiselect/rank fields. Example: If Alice selected
	//                  [Apples, Bananas] and Bob selected [Bananas, Oranges], Alice querying
	//                  Bob's values would only see [Bananas]).  Ranks will their own ranks and lower.
	AccessMode string
}

// fieldDefinitions contains all user attribute fields this plugin creates.
// These are per-user metadata fields stored in the access_control property
// group, so they appear on user profiles and can also be referenced from
// attribute-based access control (ABAC) policy rules. This plugin ensures
// these fields exist on startup and syncs external data into them.
//
// Access Control Examples:
// The fields below demonstrate the three available access control modes. These are examples
// showing what's possible - customize them based on your privacy and security requirements.
//
// All fields are marked as "protected" (see createField function), which means:
//   - Only this plugin can modify the field structure (add/remove options, change types)
//   - Only this plugin can write values (users and admins cannot manually edit)
//   - Access modes control read permissions (who can see the data)
var fieldDefinitions = []fieldDefinition{
	{
		// Public Access Example: Job titles are visible to everyone in the organization
		Name:        "job_title",
		DisplayName: "Job Title",
		Type:        model.PropertyFieldTypeText,
		AccessMode:  model.PropertyAccessModePublic,
	},
	{
		// Shared-Only Access Example: Users can only see programs they have in common
		// If viewing another user's profile, you'll only see programs you're both in
		Name:        "programs",
		DisplayName: "Programs",
		Type:        model.PropertyFieldTypeMultiselect,
		Options: []model.CustomProfileAttributesSelectOption{
			{Name: "Apples"},
			{Name: "Oranges"},
			{Name: "Lemons"},
			{Name: "Grapes"},
		},
		AccessMode: model.PropertyAccessModeSharedOnly,
	},
	{
		// Shared-Only Access Example using the Rank type (requires Mattermost server v11.9 or later - see release notes).
		// Rank types are similar to Select, with the addition that each value includes a numerical representation that
		// can support inequality comparisons (e.g. Clearance >= "Secret")
		Name:        "clearance",
		DisplayName: "Clearance",
		Type:        model.PropertyFieldTypeRank,
		Options: []model.CustomProfileAttributesSelectOption{
			{Name: "CUI", Rank: model.NewPointer(1)},
			{Name: "Confidential", Rank: model.NewPointer(2)},
			{Name: "Secret", Rank: model.NewPointer(3)},
			{Name: "Top Secret", Rank: model.NewPointer(4)},
		},
		AccessMode: model.PropertyAccessModeSharedOnly,
	},
	{
		// Source-Only Access Example: Start dates are private - only this plugin can read them
		// Useful for data that should be synchronized but not visible to users or other systems
		Name:        "start_date",
		DisplayName: "Start Date",
		Type:        model.PropertyFieldTypeDate,
		AccessMode:  model.PropertyAccessModeSourceOnly,
	},
}

var fieldTypesWithOptions = []model.PropertyFieldType{
	model.PropertyFieldTypeSelect,
	model.PropertyFieldTypeMultiselect,
	model.PropertyFieldTypeRank,
}

func getRankFieldNames() []string {
	var rankFields []string
	for _, field := range fieldDefinitions {
		if field.Type == model.PropertyFieldTypeRank {
			rankFields = append(rankFields, field.Name)
		}
	}
	return rankFields
}

// updateField updates an existing user attribute field to match the definition.
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
	existingField.Attrs[model.PropertyFieldAttrVisibility] = model.PropertyFieldVisibilityAlways
	existingField.Attrs[model.PropertyFieldAttrDisplayName] = def.DisplayName
	existingField.Attrs[model.PropertyAttrsProtected] = true
	existingField.Attrs[model.PropertyAttrsAccessMode] = def.AccessMode
	// See createField for why all three permission levels are set to sysadmin.
	sysadmin := model.PermissionLevelSysadmin
	existingField.PermissionField = &sysadmin
	existingField.PermissionValues = &sysadmin
	existingField.PermissionOptions = &sysadmin

	if slices.Contains(fieldTypesWithOptions, def.Type) {
		// Build options array with name only - Mattermost will generate IDs
		options, err := buildOptionsArr(def)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to update existing field %s", def.Name)
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

func buildOptionsArr(def fieldDefinition) ([]interface{}, error) {
	options := make([]interface{}, len(def.Options))
	for i, option := range def.Options {
		attrs := map[string]interface{}{"name": option.Name}
		// Don't forget to add in the option's rank value for Rank fields types
		if def.Type == model.PropertyFieldTypeRank {
			if option.Rank == nil {
				return nil, fmt.Errorf("encountered a missing rank for existing field %s", def.Name)
			}
			attrs["rank"] = *option.Rank
		}
		options[i] = attrs
	}
	return options, nil
}

// createField creates a new user attribute field from the definition.
// Returns the newly created field.
func createField(
	client *pluginapi.Client,
	groupID string,
	def fieldDefinition,
) (*model.PropertyField, error) {
	client.Log.Info("Field does not exist, creating", "name", def.Name)

	// These three permission levels describe who, role-wise, can edit the
	// field definition (PermissionField), write a user's value
	// (PermissionValues), or change the multiselect options (PermissionOptions).
	//
	// For this plugin they're largely a formality: every field we create is
	// protected, so only the plugin can write through the source_plugin_id
	// mechanism, and source_only/shared_only access modes already restrict
	// who can read the values — even admins can't see source_only fields.
	// Mattermost also pins PermissionField and PermissionOptions to sysadmin
	// itself for any field in the access_control group, so those two are
	// truly no-ops here.
	//
	// Even so, our shared_only field forces us to set PermissionValues. The
	// default for user fields lets members edit their own value, and
	// Mattermost rejects that combined with shared_only — if anyone could
	// pick any value, they could fake having something in common with
	// anyone. Setting PermissionValues to sysadmin clears that check. We
	// set the other two to sysadmin alongside it so all three read the same
	// way.
	sysadmin := model.PermissionLevelSysadmin

	field := &model.PropertyField{
		// ID left empty - Mattermost will auto-generate
		GroupID:           groupID,
		Name:              def.Name,
		Type:              def.Type,
		PermissionField:   &sysadmin,
		PermissionValues:  &sysadmin,
		PermissionOptions: &sysadmin,

		// ObjectType declares what kind of object this field describes. This is a
		// user attribute sync plugin, so every field describes users and we pin
		// ObjectType to "user". Mattermost uses this to route field queries — for
		// example, the user-profile UI asks for fields with ObjectType=user, and
		// ABAC policy evaluation looks up user.attributes.<field> against the
		// same set.
		ObjectType: model.PropertyFieldObjectTypeUser,

		// TargetType declares the scope at which the field definition lives.
		// "system" means the field is defined once globally and applies to every
		// user on the server. The other options ("team", "channel") would scope
		// the field to a specific team or channel, which is not what we want for
		// org-wide profile attributes. With TargetType=system, TargetID must be
		// empty (the system has no per-entity ID).
		TargetType: string(model.PropertyFieldTargetLevelSystem),

		Attrs: model.StringInterface{
			// DisplayName is the user-facing label rendered in profile cards and the
			// System Console. Mattermost's Name field is a CEL identifier and can't
			// contain spaces or punctuation, so anything human-readable lives here.
			model.PropertyFieldAttrDisplayName: def.DisplayName,

			// Visibility controls whether values appear in the UI (user profiles/cards).
			// This does NOT affect data access via API - use AccessMode for that.
			// "Always" makes values visible in the UI. "Hidden" hides them from UI but
			// data can still be retrieved via API (subject to AccessMode permissions).
			model.PropertyFieldAttrVisibility: model.PropertyFieldVisibilityAlways,

			// Protected means only this plugin can:
			//   - Modify field structure (add/remove options, change field type)
			//   - Write/update values
			// This prevents users and admins from manually editing data that should be
			// synchronized from an external source. Required for non-public access modes.
			model.PropertyAttrsProtected: true,

			// AccessMode controls read permissions (who can see values via API and UI).
			// Works in conjunction with "protected" to provide complete access control.
			// See fieldDefinition.AccessMode for details on the three modes.
			model.PropertyAttrsAccessMode: def.AccessMode,
		},
	}

	// Multiselect fields need their options defined
	if slices.Contains(fieldTypesWithOptions, def.Type) {
		// Build options array with name only - Mattermost will generate IDs
		options, err := buildOptionsArr(def)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create field %s", def.Name)
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

// isFieldOwnedByPlugin checks if an existing field is owned and managed by this plugin
// by verifying the source_plugin_id attribute matches our plugin ID.
func isFieldOwnedByPlugin(
	client *pluginapi.Client,
	existingField *model.PropertyField,
	pluginID string,
	def fieldDefinition,
) bool {
	sourcePluginID, hasSource := existingField.Attrs[model.PropertyAttrsSourcePluginID]
	if !hasSource {
		// No source plugin ID - field was created by admin or other means
		client.Log.Error("Field already exists but has no source_plugin_id (likely created by admin)",
			"field_name", def.Name,
			"field_id", existingField.ID)
		return false
	}

	sourceID, ok := sourcePluginID.(string)
	if !ok || sourceID != pluginID {
		// Field is owned by a different plugin
		client.Log.Error("Field already exists but is owned by another plugin",
			"field_name", def.Name,
			"field_id", existingField.ID,
			"owner_plugin_id", sourceID)
		return false
	}

	// Source plugin ID matches ours
	return true
}

// syncSingleField ensures a single user attribute field exists and matches the definition.
// Updates the cache with field and option IDs. Returns the field ID or error.
func syncSingleField(
	client *pluginapi.Client,
	groupID string,
	pluginID string,
	def fieldDefinition,
	cache *FieldIDCache,
) (string, error) {
	// Try to get existing field
	existingField, err := client.Property.GetPropertyFieldByName(groupID, "", def.Name)

	var field *model.PropertyField
	if err == nil && existingField != nil {
		// Field exists - verify we own it before attempting to update
		if !isFieldOwnedByPlugin(client, existingField, pluginID, def) {
			return "", errors.Errorf(
				"field %s already exists but is not managed by this plugin",
				def.Name)
		}

		// Field exists and we own it - update it
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
	cache.FieldNameToID[def.Name] = field.ID

	// For supported field types, extract option IDs
	if slices.Contains(fieldTypesWithOptions, def.Type) && len(def.Options) > 0 {
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

// extractOptionIDs extracts option IDs from a field into the cache (if applicable).
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

	// Build option name to ID mapping for all supported fields
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

// SyncFields ensures all user attribute fields exist and match the definitions.
// Returns a FieldIDCache containing mappings from external names to Mattermost-generated IDs.
//
//nolint:revive
func SyncFields(client *pluginapi.Client, groupID string, pluginID string) (*FieldIDCache, error) {
	client.Log.Info("Syncing field definitions", "field_count", len(fieldDefinitions))

	cache := &FieldIDCache{
		FieldNameToID:  make(map[string]string),
		OptionNameToID: make(map[string]string),
	}

	var failedFields []string

	for _, def := range fieldDefinitions {
		_, err := syncSingleField(client, groupID, pluginID, def, cache)
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
