# choice

A Go project.

## Getting Started

### Prerequisites

- Go 1.19 or later

### Installation

```bash
git clone choice
cd choice
go mod tidy
```

### Usage

```bash
go run main.go
```

### Build

```bash
go build -o choice
./choice
```

### Testing

```bash
go test ./...
```

### Benchmarks

```bash
# 全ベンチマーク実行
go test -bench=. -benchmem ./pkg/buffer/...

# 特定のベンチマークのみ
go test -bench=BenchmarkBufferWrite -benchmem ./pkg/buffer/...

# 複数回実行して安定性を確認
go test -bench=. -benchmem -count=5 ./pkg/buffer/...

# CPUプロファイル付き
go test -bench=BenchmarkBufferWrite -benchmem -cpuprofile=cpu.prof ./pkg/buffer/...

# メモリプロファイル付き
go test -bench=BenchmarkBufferWrite -benchmem -memprofile=mem.prof ./pkg/buffer/...
```

## License

This project is licensed under the MIT License.
