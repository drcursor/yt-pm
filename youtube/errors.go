package youtube

import "fmt"

// APIError represents an application-level error returned inside an HTTP 200
// response body, e.g. {"error": {"code": 400, "message": "...", "status": "..."}}.
type APIError struct {
	Code    int
	Message string
	Status  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("youtube API error %d (%s): %s", e.Code, e.Status, e.Message)
}

// AuthError indicates that the request was rejected due to authentication
// failure (HTTP 401) or an expired / missing session.
type AuthError struct {
	HTTPStatus int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("youtube auth error (HTTP %d): %s", e.HTTPStatus, e.Message)
}

// NotFoundError is returned when a requested resource (playlist, video) does
// not exist or is inaccessible.
type NotFoundError struct {
	Resource string // e.g. "playlist PLxxxxxx" or "video dQw4w9WgXcQ"
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("youtube: not found: %s", e.Resource)
}

// MutationError is returned when a mutation operation (add/remove) did not
// return status "SUCCEEDED".
type MutationError struct {
	Operation string // e.g. "AddVideo", "RemoveVideo"
	Status    string // the status field value, or "<missing>" if absent
}

func (e *MutationError) Error() string {
	return fmt.Sprintf("youtube: %s did not succeed: status=%q", e.Operation, e.Status)
}
