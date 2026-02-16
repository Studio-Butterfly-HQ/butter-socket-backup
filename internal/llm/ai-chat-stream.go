package llm

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

func StreamButterAI(
	ctx context.Context,
	query string,
	onToken func(token string),
) error {

	client := openai.NewClient(
		option.WithAPIKey(LLM_KEY),
	)

	stream := client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model: openai.ChatModelGPT4,
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(query),
		},
	})
	defer stream.Close()

	for stream.Next() {
		event := stream.Current()

		if event.Type == "response.output_text.delta" {
			onToken(event.Delta)
		}
	}

	if err := stream.Err(); err != nil {
		return err
	}

	return nil
}
