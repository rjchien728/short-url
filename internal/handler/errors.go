package handler

// errorResponse is the standard JSON error body returned by all handlers.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
