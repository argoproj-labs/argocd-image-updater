# What lives here

The `test/` directory does not contain any tests, but all fixtures and data
for running the unit tests.

Do not add unit tests here. If a test-specific method would be useful to more
than one package's unit test, add it to the `fixture` package. Methods defined
as fixture are allowed to `panic()`, so they must not be used in code outside
the unit tests.
