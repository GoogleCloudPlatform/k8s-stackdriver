# Overview

web-echo is a simple web server that returns the same response on every GET endpoint. The response can be modified.

# Usage

Send a POST request to /response to set a new response to the raw payload. E.g. `curl -d $'Hello world\n' localhost:8080/response`

You can also set the default response from a file via the `-from-file` flag.
