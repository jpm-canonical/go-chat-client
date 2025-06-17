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

func main() {
	modelName := os.Getenv("MODEL_NAME")
	reasoningModel := os.Getenv("REASONING_MODEL") == "True"

	client := openai.NewClient()

	param := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{},
		Seed:     openai.Int(0),
		Model:    modelName,
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
		appendParam := processStream(stream, reasoningModel)

		param.Messages = append(param.Messages, appendParam)
		fmt.Println()
		fmt.Println()
	}
}

func processStream(stream *ssestream.Stream[openai.ChatCompletionChunk], printThinking bool) openai.ChatCompletionMessageParamUnion {

	// optionally, an accumulator helper can be used
	acc := openai.ChatCompletionAccumulator{}

	thinking := printThinking

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
