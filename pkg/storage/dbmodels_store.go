package storage

type DbModelsStore interface {
	DbModel(id string) DbModelStore
}

type DbModelStore interface {
	ProjectStoreRef
	ID() string
	DbModels() DbModelsStore
}
