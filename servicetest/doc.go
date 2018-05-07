/*
Package servicetest contains tools and tests for the service and
servicemgr packages.

Please do not use any of the types exposed by this package.

To run the tests with code coverage, ensure you pass the packages to
'go test' like so:

	-coverpkg=github.com/shabbyrobe/go-service,github.com/shabbyrobe/go-service/servicemgr

Because there are so many damn pieces needed to test all this stuff,
the files are rather irritatingly named test_blahblah_test.go. Maybe
I should break this into more packages so this isn't necessary but
it'll do for now.

*/
package servicetest
