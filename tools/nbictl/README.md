# NBI Command Line Interface
NBI Command Line Interface that allows users to interact with NBI API. 

## How to authenticate
To autheticate you have to pass in three flags when you run the CLI. 
* priv_key=path.key: contains a path to your private key 
* key_id=key.id: contains a key id provided by Aalyria that is associated with your service account
* email=email: contains an email address provided by Aalyria that is associated with your service account
### Note
You have to pass in those three flags everytime you make a request to get authenticated to the API. In addition to passing in those three flags you also have to pass in `file` flag that contains the pass to your textproto file that contains request message in textproto format.

## General Command Template
```
bazel run //github/tools/nbictl/cmd/nbictl -- -priv_key "path.key" -key_id "key.id" \
-email "example.com" create/update/delete/list/generate-keys -flags
```

## Examples

### How to create an entity
```
bazel run //github/tools/nbictl/cmd/nbictl -- -priv_key "path/private_key.pem" -key_id "key.id" \
-email "example.com" create -files "path/*.textproto" 
```

### How to update an entity 
```
bazel run //github/tools/nbictl/cmd/nbictl ---priv_key "path/private_key.pem" -key_id "key.id" \
-email "example.com" update -files "path/*.textproto"
```

### How to delete an entity
```
bazel run //github/tools/nbictl/cmd/nbictl -- -priv_key "path/private_key.pem" -key_id "key.id" \
-email "example.com" delete -type "entity_type" -id "entity_id" -commit_time "entity_commit_time"
```

### How to generate RSA keys
```
bazel run //github/tools/nbictl/cmd/nbictl -- -priv_key "path/private_key.pem" -key_id "key.id" \
-email "example.com" generate-keys -org "example.org"
```