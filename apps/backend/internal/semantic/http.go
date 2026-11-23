package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxProviderBody = 1 << 20

// HTTPAdapter is the standard-library provider transport. Token is sent
// only when the caller set it; the adapter never reads credentials from
// the environment or from the local filesystem.
type HTTPAdapter struct {
	URL    string
	Model  string
	Token  string
	Client *http.Client
}

type providerRequest struct {
	Model     string            `json:"model"`
	Messages  []providerMessage `json:"messages"`
	MaxTokens int64             `json:"max_tokens"`
}

type providerMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type providerResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

// Complete posts one JSON chat request. A non-HTTP URL is rejected before
// the transport runs, so a file URL cannot attach a local credential.
func (h *HTTPAdapter) Complete(ctx context.Context, req CompletionRequest) (ModelResponse, error) {
	if h == nil {
		return ModelResponse{}, fmt.Errorf("semantic: missing http adapter")
	}
	parsed, err := url.Parse(h.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ModelResponse{}, fmt.Errorf("semantic: provider url must be http or https")
	}
	payload, err := json.Marshal(providerRequest{
		Model: h.Model,
		Messages: []providerMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.UserJSON},
		},
		MaxTokens: req.MaxOutputTokens,
	})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("semantic: encode provider request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("semantic: build provider request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+h.Token)
	}
	resp, err := h.client().Do(httpReq)
	if err != nil {
		return ModelResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody+1))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("semantic: read provider response")
	}
	if len(body) > maxProviderBody {
		return ModelResponse{}, fmt.Errorf("semantic: provider response exceeds 1MiB")
	}
	if resp.StatusCode != http.StatusOK {
		return ModelResponse{}, fmt.Errorf("semantic: provider status %d", resp.StatusCode)
	}
	var decoded providerResponse
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Choices) == 0 {
		return ModelResponse{}, fmt.Errorf("semantic: provider response has no content")
	}
	out := ModelResponse{
		Content:   decoded.Choices[0].Message.Content,
		RequestID: decoded.ID,
	}
	if decoded.Usage != nil {
		out.TokensKnown = true
		out.InputTokens = decoded.Usage.PromptTokens
		out.OutputTokens = decoded.Usage.CompletionTokens
	}
	return out, nil
}

func (h *HTTPAdapter) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	// ProxyFromEnvironment would forward the prompt to whatever proxy the
	// process inherited. A fresh transport does not.
	return &http.Client{Transport: &http.Transport{Proxy: nil}}
}
