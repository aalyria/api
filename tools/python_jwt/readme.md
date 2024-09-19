# How to ude the `generate_jwt.py` file
The `generate_jwt.py` generates two JWTS:
- One "standard"
- One for the reflections service (used to interact with the APIs by using `grpcurl`)

## How to use it
First, you have to have your private key file in the same folder of the Python file and call it `private_key.pem`.

Then, install the requirements:
```bash
pip install -r requirements.txt
```

