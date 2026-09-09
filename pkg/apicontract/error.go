package apicontract

import "fmt"

// ErrorCode is the closed set of error codes api-contract.md "Security and
// errors" declares, each mapped to its exact HTTP status.
type ErrorCode string

const (
	ErrCodeInvalidRequest                ErrorCode = "INVALID_REQUEST"
	ErrCodeTypeMismatch                  ErrorCode = "TYPE_MISMATCH"
	ErrCodeMissingParameter              ErrorCode = "MISSING_PARAMETER"
	ErrCodeAmbiguousBinding              ErrorCode = "AMBIGUOUS_BINDING"
	ErrCodeTargetRequired                ErrorCode = "TARGET_REQUIRED"
	ErrCodeUnauthenticated               ErrorCode = "UNAUTHENTICATED"
	ErrCodeAccessDenied                  ErrorCode = "ACCESS_DENIED"
	ErrCodeUnsupportedProtectedExecution ErrorCode = "UNSUPPORTED_PROTECTED_EXECUTION"
	ErrCodeNotFound                      ErrorCode = "NOT_FOUND"
	ErrCodeStaleContext                  ErrorCode = "STALE_CONTEXT"
	ErrCodeResponseTooLarge              ErrorCode = "RESPONSE_TOO_LARGE"
	ErrCodeSourceUnavailable             ErrorCode = "SOURCE_UNAVAILABLE"
	ErrCodeTimeout                       ErrorCode = "TIMEOUT"
)

// errorCodeHTTPStatus is the exact code -> status mapping from "HTTP 400
// covers INVALID_REQUEST, TYPE_MISMATCH, MISSING_PARAMETER, AMBIGUOUS_BINDING
// and TARGET_REQUIRED; 401 UNAUTHENTICATED; 403 ACCESS_DENIED or
// UNSUPPORTED_PROTECTED_EXECUTION; 404 NOT_FOUND...; 409 STALE_CONTEXT; 413
// RESPONSE_TOO_LARGE; 503 SOURCE_UNAVAILABLE; 504 TIMEOUT."
var errorCodeHTTPStatus = map[ErrorCode]int{
	ErrCodeInvalidRequest:                400,
	ErrCodeTypeMismatch:                  400,
	ErrCodeMissingParameter:              400,
	ErrCodeAmbiguousBinding:              400,
	ErrCodeTargetRequired:                400,
	ErrCodeUnauthenticated:               401,
	ErrCodeAccessDenied:                  403,
	ErrCodeUnsupportedProtectedExecution: 403,
	ErrCodeNotFound:                      404,
	ErrCodeStaleContext:                  409,
	ErrCodeResponseTooLarge:              413,
	ErrCodeSourceUnavailable:             503,
	ErrCodeTimeout:                       504,
}

// Valid reports whether c is one of the closed set of error codes.
func (c ErrorCode) Valid() bool {
	_, ok := errorCodeHTTPStatus[c]
	return ok
}

// HTTPStatus returns c's exact HTTP status, or 0 for an unknown code.
func (c ErrorCode) HTTPStatus() int {
	return errorCodeHTTPStatus[c]
}

// TargetOption is one authorized eligible source offered on a
// TARGET_REQUIRED error - "the same authorized target options in
// error.targets, never hidden source IDs."
type TargetOption struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

func (t TargetOption) Validate() error {
	if err := requireNonEmpty("source", t.Source); err != nil {
		return err
	}
	return requireNonEmpty("label", t.Label)
}

// ErrorBody is the exact shape of every error's "error" field. "Messages and
// IDs must not reveal protected values or credentials. Tests assert both
// status and code, not English wording." api-contract.md "Security and
// errors".
type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Field     string         `json:"field,omitempty"`
	RequestID string         `json:"requestId"`
	Targets   []TargetOption `json:"targets,omitempty"`
}

// Validate enforces Code is one of the closed ErrorCode set, Message and
// RequestID are required, and Targets is populated only when Code is
// TARGET_REQUIRED - "Only TARGET_REQUIRED may include authorized target
// options."
func (e ErrorBody) Validate() error {
	if !ErrorCode(e.Code).Valid() {
		return &ValidationError{Field: "code", Message: fmt.Sprintf("unknown error code %q", e.Code)}
	}
	if err := requireNonEmpty("message", e.Message); err != nil {
		return err
	}
	if err := requireNonEmpty("requestId", e.RequestID); err != nil {
		return err
	}
	if len(e.Targets) > 0 && ErrorCode(e.Code) != ErrCodeTargetRequired {
		return &ValidationError{Field: "targets", Message: "only TARGET_REQUIRED may include target options"}
	}
	for i, t := range e.Targets {
		if err := t.Validate(); err != nil {
			return &ValidationError{Field: "targets", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// ErrorEnvelope is the exact shape of every error response.
// "{error:{code:string,message:string,field?:string,requestId:string,
// targets?:{source:string,label:string}[]}}" api-contract.md "Security and
// errors".
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func (e ErrorEnvelope) Validate() error {
	return e.Error.Validate()
}
