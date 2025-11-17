package sync

import (
	"encoding/json"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// testFieldIDCache creates a FieldIDCache with test data for use in tests
func testFieldIDCache() *FieldIDCache {
	return &FieldIDCache{
		FieldNameToID: map[string]string{
			"job_title":  "test_field_id_1",
			"programs":   "test_field_id_2",
			"start_date": "test_field_id_3",
		},
		OptionNameToID: map[string]string{
			"Apples":  "test_opt_id_apples",
			"Oranges": "test_opt_id_oranges",
			"Lemons":  "test_opt_id_lemons",
		},
	}
}

func TestFormatStringValue(t *testing.T) {
	t.Run("simple text value", func(t *testing.T) {
		result, err := formatStringValue("Engineering")
		require.NoError(t, err)

		// Verify it's properly JSON-encoded (with quotes)
		assert.Equal(t, json.RawMessage(`"Engineering"`), result)

		// Verify it unmarshals correctly
		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "Engineering", decoded)
	})

	t.Run("date string value", func(t *testing.T) {
		result, err := formatStringValue("2023-01-15")
		require.NoError(t, err)

		// Verify it's properly JSON-encoded
		assert.Equal(t, json.RawMessage(`"2023-01-15"`), result)

		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "2023-01-15", decoded)
	})

	t.Run("empty string", func(t *testing.T) {
		result, err := formatStringValue("")
		require.NoError(t, err)

		// Empty string should be encoded as ""
		assert.Equal(t, json.RawMessage(`""`), result)

		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "", decoded)
	})

	t.Run("string with special characters", func(t *testing.T) {
		result, err := formatStringValue(`He said "hello"`)
		require.NoError(t, err)

		// Quotes should be escaped
		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, `He said "hello"`, decoded)
	})

	t.Run("string with newlines and tabs", func(t *testing.T) {
		result, err := formatStringValue("Line 1\nLine 2\tTabbed")
		require.NoError(t, err)

		// Special characters should be escaped
		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "Line 1\nLine 2\tTabbed", decoded)
	})

	t.Run("string with backslashes", func(t *testing.T) {
		result, err := formatStringValue(`C:\Users\John`)
		require.NoError(t, err)

		// Backslashes should be escaped
		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, `C:\Users\John`, decoded)
	})

	t.Run("unicode characters", func(t *testing.T) {
		result, err := formatStringValue("Hello 世界 🌍")
		require.NoError(t, err)

		var decoded string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, "Hello 世界 🌍", decoded)
	})
}

func TestFormatMultiselectValue(t *testing.T) {
	cache := testFieldIDCache()

	t.Run("multiple option values", func(t *testing.T) {
		result, err := formatMultiselectValue("programs", []string{"Apples", "Oranges"}, cache)
		require.NoError(t, err)

		// Verify it's properly JSON-encoded array of IDs
		var decoded []string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, []string{"test_opt_id_apples", "test_opt_id_oranges"}, decoded)
	})

	t.Run("single option value", func(t *testing.T) {
		result, err := formatMultiselectValue("programs", []string{"Lemons"}, cache)
		require.NoError(t, err)

		var decoded []string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, []string{"test_opt_id_lemons"}, decoded)
	})

	t.Run("empty array", func(t *testing.T) {
		result, err := formatMultiselectValue("programs", []string{}, cache)
		require.NoError(t, err)

		// Empty array should be encoded as []
		assert.Equal(t, json.RawMessage(`[]`), result)

		var decoded []string
		err = json.Unmarshal(result, &decoded)
		require.NoError(t, err)
		assert.Equal(t, []string{}, decoded)
	})

	t.Run("unknown option returns error", func(t *testing.T) {
		_, err := formatMultiselectValue("programs", []string{"UnknownProgram"}, cache)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown option")
		assert.Contains(t, err.Error(), "UnknownProgram")
	})

	t.Run("unknown option for any field returns error", func(t *testing.T) {
		_, err := formatMultiselectValue("not_programs", []string{"Value1"}, cache)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown option")
		assert.Contains(t, err.Error(), "Value1")
	})
}

func TestBuildPropertyValues(t *testing.T) {
	groupID := "test-group-id"
	user := &model.User{
		Id:    "user123",
		Email: "test@example.com",
	}
	cache := testFieldIDCache()

	t.Run("builds values for all field types", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{
			"email":      "test@example.com", // Should be skipped
			"job_title":  "Software Engineer",
			"start_date": "2023-01-15",
			"programs":   []interface{}{"Apples", "Oranges"},
		}

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 3) // email excluded

		// Verify all values have correct structure
		for _, v := range values {
			assert.Equal(t, groupID, v.GroupID)
			assert.Equal(t, "user", v.TargetType)
			assert.Equal(t, user.Id, v.TargetID)
			assert.NotEmpty(t, v.FieldID)
			assert.NotEmpty(t, v.Value)
		}
	})

	t.Run("handles string array for multiselect", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{
			"email":    "test@example.com",
			"programs": []string{"Apples", "Lemons"},
		}

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 1)

		// Verify multiselect was formatted correctly
		var optionIDs []string
		err = json.Unmarshal(values[0].Value, &optionIDs)
		require.NoError(t, err)
		assert.Equal(t, []string{"test_opt_id_apples", "test_opt_id_lemons"}, optionIDs)
	})

	t.Run("skips email field", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{
			"email": "test@example.com",
		}

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 0)
	})

	t.Run("skips unknown field", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{
			"email":         "test@example.com",
			"unknown_field": "value",
			"job_title":     "Software Engineer",
		}

		// Expect log warning for unknown field
		api.On("LogWarn", "Unknown field name, skipping",
			"field_name", "unknown_field",
			"user_email", "test@example.com")

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 1) // Only job_title

		api.AssertExpectations(t)
	})

	t.Run("skips field with unsupported type", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{
			"email":     "test@example.com",
			"job_title": 123, // Unsupported type
		}

		// Expect log warning for unsupported type
		api.On("LogWarn", "Unsupported field value type, skipping field",
			"field_name", "job_title",
			"user_email", "test@example.com",
			"value_type", "int")

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 0)

		api.AssertExpectations(t)
	})

	t.Run("handles empty attributes", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		userAttrs := map[string]interface{}{}

		values, err := buildPropertyValues(client, user, groupID, userAttrs, cache)
		require.NoError(t, err)
		assert.Len(t, values, 0)
	})
}

func TestSyncUsers(t *testing.T) {
	groupID := "test-group-id"
	cache := testFieldIDCache()

	t.Run("successfully syncs multiple users", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		user1 := &model.User{Id: "user1", Email: "user1@example.com"}
		user2 := &model.User{Id: "user2", Email: "user2@example.com"}

		api.On("GetUserByEmail", "user1@example.com").Return(user1, nil)
		api.On("GetUserByEmail", "user2@example.com").Return(user2, nil)
		api.On("UpsertPropertyValues", mock.Anything).Return([]*model.PropertyValue{}, nil)
		api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

		users := []map[string]interface{}{
			{
				"email":     "user1@example.com",
				"job_title": "Software Engineer",
				"programs":  []interface{}{"Apples"},
			},
			{
				"email":     "user2@example.com",
				"job_title": "Sales Manager",
				"programs":  []interface{}{"Lemons"},
			},
		}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})

	t.Run("skips user without email", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		user1 := &model.User{Id: "user1", Email: "user1@example.com"}

		api.On("GetUserByEmail", "user1@example.com").Return(user1, nil)
		api.On("UpsertPropertyValues", mock.Anything).Return([]*model.PropertyValue{}, nil)
		api.On("LogWarn", "User object missing email field, skipping")
		api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

		users := []map[string]interface{}{
			{
				"job_title": "Software Engineer", // Missing email
			},
			{
				"email":     "user1@example.com",
				"job_title": "Sales Manager",
			},
		}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})

	t.Run("skips user not found in Mattermost", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		user2 := &model.User{Id: "user2", Email: "user2@example.com"}

		notFoundErr := model.NewAppError("GetUserByEmail", "app.user.get_by_email.app_error", nil, "", 404)
		api.On("GetUserByEmail", "notfound@example.com").Return(nil, notFoundErr)
		api.On("GetUserByEmail", "user2@example.com").Return(user2, nil)
		api.On("UpsertPropertyValues", mock.Anything).Return([]*model.PropertyValue{}, nil)
		api.On("LogWarn", "User not found by email, skipping",
			"email", "notfound@example.com",
			"error", mock.Anything) // Accept any error string
		api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

		users := []map[string]interface{}{
			{
				"email":     "notfound@example.com",
				"job_title": "Software Engineer",
			},
			{
				"email":     "user2@example.com",
				"job_title": "Sales Manager",
			},
		}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})

	t.Run("skips user with empty attributes", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		user1 := &model.User{Id: "user1", Email: "user1@example.com"}

		api.On("GetUserByEmail", "user1@example.com").Return(user1, nil)
		api.On("LogDebug", "No property values to sync for user", "email", "user1@example.com")

		users := []map[string]interface{}{
			{
				"email": "user1@example.com", // Only email, no other attributes
			},
		}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})

	t.Run("continues sync when upsert fails for one user", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		user1 := &model.User{Id: "user1", Email: "user1@example.com"}
		user2 := &model.User{Id: "user2", Email: "user2@example.com"}

		api.On("GetUserByEmail", "user1@example.com").Return(user1, nil)
		api.On("GetUserByEmail", "user2@example.com").Return(user2, nil)

		// First user upsert fails
		api.On("UpsertPropertyValues", mock.MatchedBy(func(values []*model.PropertyValue) bool {
			return len(values) > 0 && values[0].TargetID == "user1"
		})).Return(nil, assert.AnError).Once()

		// Second user upsert succeeds
		api.On("UpsertPropertyValues", mock.MatchedBy(func(values []*model.PropertyValue) bool {
			return len(values) > 0 && values[0].TargetID == "user2"
		})).Return([]*model.PropertyValue{}, nil).Once()

		api.On("LogError", "Failed to upsert property values, skipping user",
			"user_email", "user1@example.com",
			"value_count", 1,
			"error", mock.Anything) // Accept any error string
		api.On("LogDebug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

		users := []map[string]interface{}{
			{
				"email":     "user1@example.com",
				"job_title": "Software Engineer",
			},
			{
				"email":     "user2@example.com",
				"job_title": "Sales Manager",
			},
		}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})

	t.Run("handles empty users array", func(t *testing.T) {
		api := &plugintest.API{}
		client := pluginapi.NewClient(api, &plugintest.Driver{})

		users := []map[string]interface{}{}

		err := SyncUsers(client, groupID, users, cache)
		require.NoError(t, err)

		api.AssertExpectations(t)
	})
}
