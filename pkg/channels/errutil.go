package channels

import (
	"fmt"
	"net/http"
	"strings"
)

// ClassifySendError wraps a raw error with the appropriate sentinel based on
// an HTTP status code. Channels that perform HTTP API calls should use this
// in their Send path.
func ClassifySendError(statusCode int, rawErr error) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", ErrRateLimit, rawErr)
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrAuthFailed, rawErr)
	case statusCode >= 500:
		return fmt.Errorf("%w: %v", ErrTemporary, rawErr)
	case statusCode >= 400:
		return fmt.Errorf("%w: %v", ErrSendFailed, rawErr)
	default:
		return rawErr
	}
}

// ClassifyNetError wraps a network/timeout error as ErrTemporary.
func ClassifyNetError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrTemporary, err)
}

// ClassifyAPIError classifies an error from a platform SDK by inspecting
// its message text. Use this when the channel SDK does not expose raw HTTP
// status codes. The heuristic checks for common patterns across SDKs
// (discordgo, telego, slack-go, etc.).
func ClassifyAPIError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		return fmt.Errorf("%w: %v", ErrRateLimit, err)
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "401") || strings.Contains(msg, "403"):
		return fmt.Errorf("%w: %v", ErrAuthFailed, err)
	case strings.Contains(msg, "invalid") || strings.Contains(msg, "not found") ||
		strings.Contains(msg, "bad request") || strings.Contains(msg, "400"):
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	default:
		// Default to temporary: SDK errors are often transient (timeouts,
		// connection issues, server errors) and benefit from retry.
		return fmt.Errorf("%w: %v", ErrTemporary, err)
	}
}
