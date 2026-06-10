# llm.exchange agent

Provider agent for [llm.exchange](https://github.com/guhcostan/llm-exchange-platform) — connects your local Ollama or vLLM GPU to the marketplace via WebSocket.

## Install

```bash
git clone git@github.com:guhcostan/llm-exchange-agent.git
cd llm-exchange-agent
cp config.example.yaml config.yaml
```

Edit `config.yaml`: set `provider_token` from the dashboard (starts with `ptok_`).

## Run

```bash
go run ./cmd/agent
# or: make agent   (from platform repo dev setup)
```

## Config

See `config.example.yaml`. Protocol: [llm-exchange-contracts](https://github.com/guhcostan/llm-exchange-contracts).

## License

MIT
