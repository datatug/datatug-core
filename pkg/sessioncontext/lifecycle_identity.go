package sessioncontext

// SessionLifecycleID identifies a session by its operational namespace and
// stored session ID. The namespace must come from the owning repository: its
// digest binds the mount and private parent, not the native execution target.
// Owner is authorization/index metadata and is deliberately absent from the key.
// This identity establishes neither source authority nor complete project usage.
func SessionLifecycleID(namespace NativeNamespace, sessionID string) (string, error) {
	if !validSessionLifecycleNamespace(namespace) || !safeID.MatchString(sessionID) {
		return "", ErrInvalid
	}
	return namespace.NamespaceID + "/" + sessionID, nil
}

func validSessionLifecycleNamespace(namespace NativeNamespace) bool {
	return safeID.MatchString(namespace.StorageID) && safeID.MatchString(namespace.DatabaseID) &&
		safeID.MatchString(namespace.ProjectID) && retainedDigest(namespace.NamespaceID) &&
		(namespace.StorageID == "local" && namespace.SpaceID == "" || namespace.StorageID != "local" && safeID.MatchString(namespace.SpaceID))
}
