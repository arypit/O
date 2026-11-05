package tui

import (
	"strings"
)

// friendlyError is a backend failure translated into something a person can
// act on: what happened, why, and what to do next.
type friendlyError struct {
	Title   string
	Detail  string
	Suggest string
	Level   string // provider | network | request | filesystem | command | general
}

// classifyError turns a raw provider, HTTP, JSON, filesystem or command
// failure into an explanation with a recovery suggestion. The raw backend
// message is never shown as-is.
func classifyError(raw string) friendlyError {
	msg := strings.TrimSpace(raw)
	low := strings.ToLower(msg)

	switch {
	case containsAny(low, "http 401", "unauthorized", "invalid api key", "incorrect api key", "api key not valid", "authentication"):
		return friendlyError{
			Title:   "The provider rejected the API key",
			Detail:  "The request was refused before it reached the model.",
			Suggest: "Open /providers to check the key for this provider, then try again.",
			Level:   "provider",
		}
	case containsAny(low, "http 403", "forbidden", "permission denied by the provider"):
		return friendlyError{
			Title:   "The provider refused this request",
			Detail:  "The key is valid but is not allowed to use this model or feature.",
			Suggest: "Use a different model or provider, or ask for access to this one.",
			Level:   "provider",
		}
	case containsAny(low, "http 404", "model not found", "unknown model", "no such model"):
		return friendlyError{
			Title:   "The provider does not know this model",
			Detail:  "The endpoint answered, but the requested model does not exist there.",
			Suggest: "Check the model name in /providers and pick one the provider lists.",
			Level:   "provider",
		}
	case containsAny(low, "http 429", "rate limit", "too many requests", "quota exceeded"):
		return friendlyError{
			Title:   "The provider is rate limiting requests",
			Detail:  "Too many requests arrived in a short time.",
			Suggest: "Wait a few seconds and send your request again.",
			Level:   "provider",
		}
	case containsAny(low, "http 5", "internal server error", "bad gateway", "service unavailable", "gateway timeout"):
		return friendlyError{
			Title:   "The provider is having trouble",
			Detail:  "The failure happened on the provider's side, not in your project.",
			Suggest: "Try again in a moment; if it keeps happening, switch provider in /providers.",
			Level:   "provider",
		}
	case containsAny(low, "missing", "field required", "schema", "parameters", "function") && containsAny(low, "tool", "function", "parameter", "schema"):
		return friendlyError{
			Title:   "The provider rejected the tool definitions",
			Detail:  "This endpoint does not accept the function-calling format O sends.",
			Suggest: "Switch to another provider in /providers, or update this provider's endpoint.",
			Level:   "provider",
		}
	case containsAny(low, "connection refused", "no such host", "dns", "network is unreachable", "i/o timeout", "deadline exceeded", "tls handshake", "eof"):
		return friendlyError{
			Title:   "Could not reach the provider",
			Detail:  "The request never got an answer, so nothing was sent to the model.",
			Suggest: "Check your connection and the base URL in /providers, then try again.",
			Level:   "network",
		}
	case containsAny(low, "invalid character", "unexpected end of json", "cannot unmarshal", "invalid json", "failed to decode"):
		return friendlyError{
			Title:   "The provider sent a response we could not read",
			Detail:  "The answer was not valid JSON, so it could not be turned into a message.",
			Suggest: "Try again; if it keeps happening, switch provider in /providers.",
			Level:   "request",
		}
	case containsAny(low, "context canceled", "cancelled", "canceled"):
		return friendlyError{
			Title:   "The run was cancelled",
			Detail:  "The agent stopped before it finished.",
			Suggest: "Send the request again to continue from here.",
			Level:   "general",
		}
	case containsAny(low, "outside the workspace", "sandbox"):
		return friendlyError{
			Title:   "That action is outside the workspace",
			Detail:  "O only touches files and commands inside the project folder.",
			Suggest: "Point the agent at a path inside the workspace.",
			Level:   "filesystem",
		}
	case containsAny(low, "access is denied", "permission denied") && !containsAny(low, "http 403"):
		return friendlyError{
			Title:   "The operating system denied access",
			Detail:  "The file or folder could not be opened with the current rights.",
			Suggest: "Check the permissions of that path, or run O with the right rights.",
			Level:   "filesystem",
		}
	case containsAny(low, "no such file or directory", "cannot find the file", "the system cannot find"):
		return friendlyError{
			Title:   "The file or folder was not found",
			Detail:  "The path does not exist, or it was moved since it was listed.",
			Suggest: "Check the path and try again.",
			Level:   "filesystem",
		}
	case containsAny(low, "exit status", "command failed", "signal:", "executable file not found"):
		return friendlyError{
			Title:   "A command did not finish successfully",
			Detail:  "The command ran and reported a failure.",
			Suggest: "Run it again to see the output, or adjust the command before retrying.",
			Level:   "command",
		}
	}

	return friendlyError{
		Title:   "Something went wrong",
		Detail:  cleanDetail(msg),
		Suggest: "Try again — if it keeps happening, check /providers and /settings.",
		Level:   "general",
	}
}

// cleanDetail turns a raw backend message into one short, readable line:
// no JSON blobs, no stack traces, no credentials.
func cleanDetail(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	// drop a "provider <id>: " prefix, keep the informative tail
	if i := strings.Index(msg, ": "); i >= 0 && i < 40 {
		msg = msg[i+2:]
	}
	// cut a trailing detail block that repeats the same information
	if i := strings.Index(msg, " ("); i > 0 {
		if j := strings.Index(msg[i:], ")"); j > 0 && j < 40 {
			msg = msg[:i] + msg[i+j+1:]
		}
	}
	msg = oneLine(msg, 160)
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "The operation did not finish."
	}
	return msg
}

// containsAny reports whether s contains one of the needles.
func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
