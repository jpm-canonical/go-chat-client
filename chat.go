package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
)

const (
	ModelName      = "DeepSeek-R1-Distill-Qwen-7B-ov-int4"
	ReasoningModel = true
)

func main() {
	client := openai.NewClient(
		//option.WithAPIKey("My API Key"), // defaults to env var OPENAI_API_KEY
		//option.WithBaseURL("http://localhost:8080/v3/"), // defaults to env var OPENAI_BASE_URL
	)

	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{},
		Seed:     openai.Int(0),
		Model:    ModelName,
	}

	fmt.Println("Type your prompt, then ENTER to submit. CTRL-C to quit.")

	for {
		fmt.Print("> ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		err := scanner.Err()
		if err != nil {
			log.Fatal(err)
		}

		param.Messages = append(param.Messages, openai.UserMessage(scanner.Text()))

		stream := client.Chat.Completions.NewStreaming(context.Background(), param)
		appendParam := processStream(stream)

		param.Messages = append(param.Messages, appendParam)
		fmt.Println()
		fmt.Println()
	}
}

func processStream(stream *ssestream.Stream[openai.ChatCompletionChunk]) openai.ChatCompletionMessageParamUnion {

	// optionally, an accumulator helper can be used
	acc := openai.ChatCompletionAccumulator{}

	thinking := ReasoningModel

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if content, ok := acc.JustFinishedContent(); ok {
			fmt.Printf("Content stream finished: %s", content)
		}

		// if using tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			fmt.Printf("Tool call stream finished %d: %s %s", tool.Index, tool.Name, tool.Arguments)
		}

		if refusal, ok := acc.JustFinishedRefusal(); ok {
			fmt.Printf("Refusal stream finished: %s", refusal)
		}

		// it's best to use chunks after handling JustFinished events
		if len(chunk.Choices) > 0 {
			lastChunk := chunk.Choices[0].Delta.Content

			if strings.Contains(lastChunk, "</think>") {
				// Catch end of thinking tag
				thinking = false
				fmt.Fprint(os.Stderr, lastChunk)
			} else if thinking {
				fmt.Fprint(os.Stderr, lastChunk)
			} else {
				fmt.Print(lastChunk)
			}
		}
	}

	if stream.Err() != nil {
		panic(stream.Err())
	}

	// After the stream is finished, acc can be used like a ChatCompletion
	return acc.Choices[0].Message.ToParam()
}
