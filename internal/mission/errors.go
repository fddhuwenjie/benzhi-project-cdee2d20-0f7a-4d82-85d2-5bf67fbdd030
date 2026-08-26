package mission

import "fmt"

type Error struct {
	Code    string
	Message string
	Status  int
	Details any
}

func (e *Error) Error() string { return e.Message }

func NewError(code, message string, status int) error {
	return &Error{Code: code, Message: message, Status: status}
}

var (
	ErrNotFound = &Error{Code: "mission_not_found", Message: "任务不存在", Status: 404}
	ErrConflict = &Error{Code: "revision_conflict", Message: "任务修订号冲突", Status: 409}
	ErrArchived = &Error{Code: "immutable_archive", Message: "任务已归档，不允许修改", Status: 409}
)

func Invalid(field, reason string) error {
	return &Error{Code: "invalid_request", Message: fmt.Sprintf("%s：%s", field, reason), Status: 400}
}

func Unprocessable(field, reason string, details any) error {
	return &Error{Code: "validation_failed", Message: fmt.Sprintf("%s：%s", field, reason), Status: 422, Details: details}
}

func InvalidState(status Status, action string) error {
	return &Error{Code: "invalid_state", Message: fmt.Sprintf("状态 %s 不允许执行 %s", status, action), Status: 409}
}
