package llm

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

func AskButterAI(query string) string {
	ctx := context.Background()
	client := openai.NewClient(
		option.WithAPIKey(LLM_KEY),
	)

	question := query

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(question)},
		Model: openai.ChatModelGPT4,
	})

	if err != nil {
		panic(err)
	}

	println(resp.OutputText())
	return resp.OutputText()
}
