# Keep generated development products inside the checkout.
export GOCACHE := $(CURDIR)/.cache/go-build

.PHONY: run build test vet android android-run touch clean

run:
	go run .

build:
	mkdir -p bin
	go build -o bin/timebandits .

test:
	go test ./...

vet:
	go vet ./...

android:
	./scripts/build-android.sh

android-run:
	./scripts/run-android.sh

touch:
	go run . -touch

clean:
	rm -rf bin
