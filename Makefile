SHELL := /usr/bin/env bash

.PHONY: bootstrap install-bindgen generate verify verify-checksums update-checksums clean

bootstrap:
	./scripts/bootstrap-cdk.sh

install-bindgen:
	./scripts/install-uniffi-bindgen-go.sh

generate:
	./scripts/generate-bindings.sh

verify:
	./scripts/verify-go.sh

verify-checksums:
	./scripts/verify-checksums.sh

update-checksums:
	./scripts/update-checksums.sh

clean:
	rm -rf .work bindings/cdkffi
