package incidents

import "fmt"

func (r SourceOperationRef) Validate() error {
	for _, value := range []string{r.StoreID, r.SpaceID, r.ProjectID, r.SourceID, r.ReceiptActorID, r.OperationID, r.Result.Generation} {
		if len(value) > 256 || !validSegment(value) {
			return fmt.Errorf("source-operation identity is not canonical")
		}
	}
	if r.Result.Revision < 1 || !sourceOperationHash(r.RequestSHA256) || !sourceOperationHash(r.Result.SHA256) {
		return fmt.Errorf("source-operation immutable pointer/hash is invalid")
	}
	return nil
}
func sourceOperationHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
