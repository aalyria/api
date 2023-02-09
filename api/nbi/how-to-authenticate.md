# How to authenticate

If you are calling the Spacetime APIs from a CLI tool such as `curl` or
`grpcurl`, follow these directions to acquire an ID token that can be used to
authenticate with the APIs. Be sure using your Aalyria-provided IAP client ID,
desktop client ID, and desktop client secret for `IAP_CLIENT_ID`,
`DESKTOP_CLIENT_ID` and `DESKTOP_CLIENT_SECRET`, respectively.

1. Start a local server that can echo incoming requests

   ```console
   $ nc -k -l 4444
   ```

2. Visit the following URI in your browser, being certain to replace
   `DESKTOP_CLIENT_ID` with the client ID provided by Aalyria for your instance:

   ```
   https://accounts.google.com/o/oauth2/v2/auth?client_id=DESKTOP_CLIENT_ID&response_type=code&scope=openid%20email&access_type=offline&redirect_uri=http://localhost:4444
   ```

3. In the local server output, look for the request parameters. You should see
   something like the following:
   `GET /?code=\$CODE&scope=email%20openid%20https://www.googleapis.com/auth/userinfo.email&hd=google.com&prompt=consent HTTP/1.1`
   copy the **CODE** to replace `AUTH_CODE` below. Remember to replace
   `DESKTOP_CLIENT_ID` and `DESKTOP_CLIENT_SECRET` also:

   ```sh
   curl --verbose \
      --data client_id=$DESKTOP_CLIENT_ID \
      --data client_secret=$DESKTOP_CLIENT_SECRET \
      --data code=$AUTH_CODE \
      --data redirect_uri=http://localhost:4444 \
      --data grant_type=authorization_code \
      https://oauth2.googleapis.com/token
   ```

   This call returns a JSON object with a `refresh_token` field that you can
   save as a login token to access the application.

4. Finally, exchange the `refresh_token` for an `id_token`.

   ```sh
   curl --verbose \
      --data client_id=$DESKTOP_CLIENT_ID \
      --data client_secret=$DESKTOP_CLIENT_SECRET \
      --data refresh_token=$REFRESH_TOKEN \
      --data grant_type=refresh_token \
      --data audience=$IAP_CLIENT_ID \
      https://oauth2.googleapis.com/token
   ```

   This call returns a JSON object with an `id_token` field. This token can
   then be used in HTTP authorization request headers by using the `-H`
   parameter of `curl` or `grpcurl`: `-H "Authorization: Bearer $ID_TOKEN"`.
