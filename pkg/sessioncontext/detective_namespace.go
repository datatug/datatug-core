package sessioncontext

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/dal-go/record"
)

// NativeNamespace is the private operational authority, not the source engine.
type NativeNamespace struct {
	StorageID   string `json:"storageId"`
	SpaceID     string `json:"spaceId,omitempty"`
	DatabaseID  string `json:"databaseId"`
	ProjectID   string `json:"projectId"`
	NamespaceID string `json:"namespaceId"`
}

// NewNativePinRepository explicitly binds pinning to the real mount and an owned
// repository parent. Ordinary NewDALRepository does not enable native pinning.
func NewNativePinRepository(mounted MountedDatabase, parent *record.Key, storage, space, project string) (*DALRepository, error) {
	if !safeID.MatchString(storage) || !safeID.MatchString(project) || storage == "local" && space != "" || storage != "local" && (!safeID.MatchString(space) || parent == nil) {
		return nil, ErrInvalid
	}
	owned, err := clonePrivateParent(parent, 0)
	if err != nil {
		return nil, err
	}
	repo, err := NewDALRepository(mounted, owned)
	if err != nil {
		return nil, err
	}
	path := [][2]string{}
	for key := owned; key != nil; key = key.Parent() {
		path = append(path, [2]string{key.Collection(), key.ID.(string)})
	}
	encoded, _ := json.Marshal(struct {
		Storage, Space, Database, Project string
		Parent                            [][2]string
	}{storage, space, repo.DatabaseID(), project, path})
	hash := sha256.Sum256(encoded)
	repo.nativeNamespace = NativeNamespace{storage, space, repo.DatabaseID(), project, hex.EncodeToString(hash[:])}
	return repo, nil
}

func clonePrivateParent(parent *record.Key, depth int) (*record.Key, error) {
	if parent == nil {
		return nil, nil
	}
	if depth >= 16 || parent.Validate() != nil {
		return nil, ErrInvalid
	}
	id, ok := parent.ID.(string)
	if !ok || len(id) == 0 || len(id) > 512 || len(parent.Collection()) > 128 {
		return nil, ErrInvalid
	}
	ancestor, err := clonePrivateParent(parent.Parent(), depth+1)
	if err != nil {
		return nil, err
	}
	return record.NewKeyWithParentAndID(ancestor, parent.Collection(), id), nil
}

func (r *DALRepository) NativeNamespace() NativeNamespace { return r.nativeNamespace }

// NewNativePinService is explicitly opt-in and remains unrouted/dormant.
func NewNativePinService(repo Repository, authorize Authorizer) (*Service, error) {
	identity, ok := repo.(interface{ NativeNamespace() NativeNamespace })
	if !ok || identity.NativeNamespace().NamespaceID == "" {
		return nil, ErrUnavailable
	}
	service, err := NewService(repo, authorize)
	if err != nil {
		return nil, err
	}
	namespace := identity.NativeNamespace()
	service.nativeNamespace = &namespace
	return service, nil
}
