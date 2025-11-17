package sync

// AttributeProvider defines the interface for data sources that provide user attributes
// to be synchronized into Mattermost's Custom Profile Attributes system.
//
// The interface is designed to be stateless from the caller's perspective - the provider
// implementation is responsible for tracking its own state (e.g., last read time, pagination
// tokens, etc.) to enable incremental synchronization.
//
// Implementations should:
//   - Return all user attribute data on the first call to GetUserAttributes()
//   - Return only changed/new user data on subsequent calls (incremental sync)
//   - Return an empty array when no changes are detected
//   - Return data as maps where keys are field names and values are field data
type AttributeProvider interface {
	// GetUserAttributes retrieves user attribute data from the external source.
	// Returns an array of user objects where each object is a map of field names to values.
	// The implementation should track state internally to support incremental synchronization.
	// Returns an empty array if no new/changed data is available.
	GetUserAttributes() ([]map[string]interface{}, error)

	// Close releases any resources held by the provider (e.g., network connections,
	// file handles). Should be called when the provider is no longer needed.
	Close() error
}
