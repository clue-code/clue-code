package model

func init() {
	RegisterProvider("openrouter", func(mc ModelConfig, apiKey string) (Client, error) {
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "https://openrouter.ai/api/v1/chat/completions"
		}
		return &genericOpenAIClient{
			httpClient: newHTTPClient(endpoint, apiKey),
			modelID:    mc.ID,
			providerID: "openrouter",
		}, nil
	})
}
