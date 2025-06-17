# Simple CLI Chat client

This Go applications is a simple chat client for an OpenAI-compatible API.

It takes user input prompts, send it to the configured server, and prints output from a stream.

Previous prompts and answers are stored as context until the application is closed.

# Configuration

These four environment variables are used by the underlying library:

* OPENAI_API_KEY
* OPENAI_ORG_ID
* OPENAI_PROJECT_ID
* OPENAI_BASE_URL

# Usage

The chat application can be run directly:
```
$ OPENAI_BASE_URL="http://my.server:8080/v3/" go run chat.go 
```

Or it can be built and the resulting binary then be run:
```
$ go build chat.go 
$ OPENAI_BASE_URL="http://katryn.local:8080/v3/" ./chat 
```

# Licensing

This application is heavily based on and inspired by the readme of
the [openai-go library](https://github.com/openai/openai-go), which is used in this project.
The library is licensed under the Apache-2.0 license. 
