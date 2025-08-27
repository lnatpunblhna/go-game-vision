package utils

import (
	"fmt"
)

// Define error types
var (
	ErrProcessNotFound      = fmt.Errorf("process not found")
	ErrWindowNotFound       = fmt.Errorf("window not found")
	ErrCaptureFailure       = fmt.Errorf("capture failed")
	ErrImageProcessing      = fmt.Errorf("image processing failed")
	ErrInvalidParameter     = fmt.Errorf("invalid parameter")
	ErrPlatformNotSupported = fmt.Errorf("platform not supported")
)

// CustomError custom error struct
type CustomError struct {
	Code    int
	Message string
	Cause   error
}

// Error implements error interface
func (e *CustomError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap returns the original error
func (e *CustomError) Unwrap() error {
	return e.Cause
}

// NewError creates a new custom error
func NewError(code int, message string, cause error) *CustomError {
	return &CustomError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// WrapError wraps an error with additional message
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// IsError checks if error matches target error
func IsError(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	return err.Error() == target.Error()
}
