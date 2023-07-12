.PHONY: protos
protos:
	mkdir -p server/internal/generated
	protoc -I=protos --go_out=server/internal/generated --go_opt=paths=source_relative \
		--go-grpc_out=server/internal/generated --go-grpc_opt=paths=source_relative \
		--js_out=import_style=commonjs:./webclient/generated \
		--grpc-web_out=import_style=commonjs,mode=grpcwebtext:./webclient/generated \
		./protos/**/*.proto

.PHONY: install-tools
install-tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

.PHONY: mock-ssl
mock-ssl:
	mkdir -p nginx/ssl
	openssl req -nodes -new -x509 -keyout nginx/ssl/key.pem -out nginx/ssl/cert.pem

.PHONY: run
run:
	docker-compose up --build
