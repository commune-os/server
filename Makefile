all: clean sqlc build 
build: 
	cd cmd/commune;go build -o ../../bin/commune
vendor: clean vendorbuild 
vendorbuild:
	go build -mod=vendor -o bin/commune cmd/commune/main.go
clean: 
	rm -f bin/commune;
sqlc:
	-sqlc generate -f db/matrix/sqlc.yaml;
deps:
	-go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest;
	-go install github.com/cortesi/modd/cmd/modd@latest;
