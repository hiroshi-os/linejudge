.PHONY: test eval run web demo tidy

test:
	go test ./...

eval:
	go run ./cmd/eval

run:
	go run ./cmd/linejudge

web:
	cd web && npm install && npm run dev

tidy:
	go mod tidy

demo: data
	bash scripts/demo.sh

data:
	mkdir -p data
