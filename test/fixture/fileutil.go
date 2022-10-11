package fixture

import "os"

// Fixture functions for tests related to files

// MustReadFile must read a file from given path. Panics if it can't.
func MustReadFile(path string) string {
	retBytes, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(retBytes)
}
