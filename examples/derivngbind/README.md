# derivngbind Example

This directory contains an example of a code generator, `derivngbind`, built using the `go-scan` library.

`derivngbind` generates a `Bind` method for Go structs. This method populates the struct's fields from HTTP request data, including:
- Path parameters
- Query parameters
- Headers
- Cookies
- Request body (JSON)

## How it Works

The generator scans struct definitions that have the `@derivng:binding` annotation in their GoDoc.
It then determines how to bind data to the struct's fields based on a combination of the struct's GoDoc and individual field tags.

**Struct-Level Annotation:**

-   `@derivng:binding`: Marks the struct for processing.
-   `@derivng:binding in:"body"`: If `in:"body"` is present in the same GoDoc line as `@derivng:binding`, the entire struct is considered a target for the JSON request body. Fields within this struct that do *not* have their own `in` tags will be populated from the JSON body based on their `json` tags.

**Field-Level Tags:**

Fields use a combination of an `in` tag to specify the source and a source-specific tag for the name.

-   **Path Parameters:**
    -   `in:"path"`: Specifies the field comes from a path parameter.
    -   `path:"<param-name>"`: Specifies the name of the path parameter.
    -   Example: `UserID string \`in:"path" path:"userID"\``
    -   *Note: Path parameter binding uses `req.PathValue("<param-name>")` and thus requires Go 1.22 or later. For older Go versions, this binding will be a placeholder.*

-   **Query Parameters:**
    -   `in:"query"`: Specifies the field comes from a URL query parameter.
    -   `query:"<param-name>"`: Specifies the name of the query parameter.
    -   Example: `Name string \`in:"query" query:"name"\``

-   **Headers:**
    -   `in:"header"`: Specifies the field comes from a request header.
    -   `header:"<header-name>"`: Specifies the name of the header.
    -   Example: `APIKey string \`in:"header" header:"X-API-Key"\``

-   **Cookies:**
    -   `in:"cookie"`: Specifies the field comes from a request cookie.
    -   `cookie:"<cookie-name>"`: Specifies the name of the cookie.
    -   Example: `SessionID string \`in:"cookie" cookie:"session_id"\``

-   **Request Body (Field-Specific):**
    -   `in:"body"`: If this tag is on a specific field, the entire JSON request body will be unmarshalled into this field. The field's type should be a struct or a pointer to a struct suitable for `json.Unmarshal`.
    -   Example: `Payload MyPayloadStruct \`in:"body"\``

**Supported Field Types:**

For path, query, header, and cookie binding, the supported field types are `string`, `int`, and `bool`.
Pointer types for these sources are a TODO. For `in:"body"` fields, any type compatible with `encoding/json` Unmarshalling is supported.

## Usage

1.  Define your structs with the `@derivng:binding` annotation and appropriate `in:` tags.
2.  Run the generator:
    ```bash
    go run main.go generator.go <path_to_your_package_with_models>
    ```
    For example:
    ```bash
    go run main.go generator.go ./testdata/simple
    ```
3.  This will generate a `<pkgname>_deriving.go` file containing the `Bind` methods.

## Running the Example

To generate the code for the example models in `testdata/simple`:

```bash
make emit-simple
```

To clean up generated files:

```bash
make clean
```
