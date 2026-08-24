package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/rcy1314/echo-noise/internal/vocechat"
)

type voceChatFailureKind uint8

const (
	voceChatFailureUnexpected voceChatFailureKind = iota
	voceChatFailureCredential
	voceChatFailureTransientSite
)

// classifyVoceChatFailure is the single policy seam between external
// VoceChat failures and runtime behavior. Only typed transport failures,
// deadlines, and explicit 5xx responses are site-transient.
func classifyVoceChatFailure(err error) voceChatFailureKind {
	if err == nil {
		return voceChatFailureUnexpected
	}
	var apiErr *vocechat.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
			return voceChatFailureCredential
		case http.StatusNotFound:
			body := strings.ToLower(strings.TrimSpace(apiErr.Body))
			for _, marker := range []string{"account", "user", "credential", "email", "账号", "账户", "用户", "邮箱"} {
				if strings.Contains(body, marker) {
					return voceChatFailureCredential
				}
			}
		}
		if apiErr.StatusCode >= http.StatusInternalServerError {
			return voceChatFailureTransientSite
		}
		return voceChatFailureUnexpected
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return voceChatFailureTransientSite
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return voceChatFailureTransientSite
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return voceChatFailureTransientSite
	}
	return voceChatFailureUnexpected
}

func isVoceChatCredentialRejected(err error) bool {
	return classifyVoceChatFailure(err) == voceChatFailureCredential
}

func isVoceChatAccountCredentialInvalid(err error) bool {
	return classifyVoceChatFailure(err) == voceChatFailureCredential
}

func isVoceChatSiteTransientFailure(err error) bool {
	return classifyVoceChatFailure(err) == voceChatFailureTransientSite
}
