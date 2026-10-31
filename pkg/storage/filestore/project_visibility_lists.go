package filestore

import (
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func filterVisibleProjectItems[T interface{ GetID() string }, S ~[]T](root, kind, prefix string, items S, options ...datatug.StoreOption) (S, error) {
	if !datatug.GetStoreOptions(options...).ActiveItemsOnly() {
		return items, nil
	}
	hidden, err := ReadProjectArchivedItems(root)
	if err != nil {
		return nil, err
	}
	visible := make(S, 0, len(items))
	for _, item := range items {
		id := item.GetID()
		if prefix != "" {
			id = prefix + "/" + id
		}
		if !hidden[datatug.ProjectObjectRef{Kind: kind, ID: id}] {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

func filterVisibleQueryTree(root string, folder *datatug.QueriesFolder, prefix string) error {
	hidden, err := ReadProjectArchivedItems(root)
	if err != nil {
		return err
	}
	filterQueryTree(folder, prefix, hidden)
	return nil
}

func filterQueryTree(folder *datatug.QueriesFolder, prefix string, hidden map[datatug.ProjectObjectRef]bool) {
	if folder == nil {
		return
	}
	items := folder.Items[:0]
	for _, item := range folder.Items {
		id := strings.TrimPrefix(prefix+"/"+item.ID, "/")
		if !hidden[datatug.ProjectObjectRef{Kind: "query", ID: id}] {
			items = append(items, item)
		}
	}
	folder.Items = items
	visible := folder.Folders[:0]
	for _, child := range folder.Folders {
		id := strings.TrimPrefix(prefix+"/"+child.ID, "/")
		if hidden[datatug.ProjectObjectRef{Kind: "query-folder", ID: id}] {
			continue
		}
		filterQueryTree(child, id, hidden)
		visible = append(visible, child)
	}
	folder.Folders = visible
}
