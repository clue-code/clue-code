package model

func init() {
	RegisterProvider("groq", func(mc ModelConfig, apiKey string) (Client, error) {
		endpoint := mc.Endpoint
		if endpoint == "" {
			endpoint = "https://api.groq.com/openai/v1/chat/completions"
		}
		return &genericOpenAIClient{
			httpClient: newHTTPClient(endpoint, apiKey),
			modelID:    mc.ID,
			providerID: "groq",
		}, nil
	})
}
