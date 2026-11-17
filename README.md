# janzeer-sdk-go

[20 JNZ] Example: sign a transfer offline, broadcast it later

## Overview

Every example signs and sends in one program. A common setup keeps the key on a machine with no network: one machine reads the nonce, the offline machine signs, a third step broadcasts. The SDK can do this today, but nothing shows it.

**What to do:** `examples/offline/main.go`, about 60 lines, in the style of `examples/quickstart`, with three clearly separated steps in one program:

1. **Online, no key:** read the network id (`client.Info`) and the sender's next nonce (`client.Nonce`). Print th

## Installation

```bash
# Clone the repository
git clone https://github.com/janzeerOrg/janzeer-sdk-go.git
cd janzeer-sdk-go

# Install dependencies (if applicable)
pip install -r requirements.txt  # Python
# or
npm install  # Node.js
```

## Usage

```bash
# Run the project
python main.py  # Python
# or
node index.js  # Node.js
```

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -m 'Add my feature'`)
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## License

See [LICENSE](LICENSE) for details.
