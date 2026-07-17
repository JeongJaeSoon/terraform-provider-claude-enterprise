// Command mockserver runs the in-memory testutil mock of the Claude
// Enterprise Admin API as a standalone HTTP server, for local dev_overrides
// smoke testing against a real terraform binary (see examples/smoke). It is
// not part of the provider build and is not used by `go install .` or the
// release pipeline.
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func main() {
	m := testutil.NewMockAdminAPI("smoke-key", []testutil.Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
	})
	m.SetPageSize(1)
	fmt.Println(m.URL())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
