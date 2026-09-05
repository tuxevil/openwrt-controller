# AI Engine

OMEGA uses the OpenAI Chat Completions contract. The same configuration supports OpenAI and compatible gateways that expose `/v1/chat/completions`.

## Configuration

Open `PLATFORM_SETTINGS` in the dashboard and set the provider base URL, model, API key and optional Sentinel system prompt. The default base URL is `https://api.openai.com/v1`.

Provider keys are encrypted before storage in `platform_settings`. Set `AI_ENGINE_ENCRYPTION_KEY` in the controller environment and keep it stable. The API never returns the stored key; the UI only reports whether one is configured.

Remote providers must use HTTPS. Plain HTTP is accepted only for `localhost`, `127.0.0.1` and `::1`, which supports a local gateway without allowing an accidental plaintext remote endpoint.

## Provider Contract

The controller sends `POST /v1/chat/completions` with an `Authorization: Bearer <provider-key>` header and a standard `model` plus `messages` JSON body. The response must contain `choices[0].message.content`; token accounting uses `usage.total_tokens` when available. ChatOps additionally requests a JSON object response.

The engine is used by ChatOps, Sentinel fleet analysis and Vault configuration audits. If no key is configured, these features fail closed with an explicit configuration error and do not silently select a local provider.
