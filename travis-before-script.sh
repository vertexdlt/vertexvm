  #!/bin/bash
# Download golangci-lint
wget -O - -q https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.21.0

# Move golangci-lint to path
sudo mv ./bin/* /usr/local/bin/

# Download and extract wabt
wget -c https://github.com/WebAssembly/wabt/releases/download/1.0.12/wabt-1.0.12-linux.tar.gz -O - | tar -xz && ls

# Move wat2wasm to path
sudo mv ./wabt-1.0.12/* /usr/local/bin/
