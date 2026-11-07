package cli

import (
	"fmt"
	"sort"
	"strings"
)

// helpTopics are the `code-central help <topic>` pages.
var helpTopics = map[string]string{
	"sandboxes": `Reaching Code Central from a sandbox or container

The service listens only on the host's loopback, localhost:7733. The
.mcp.json init writes reads CODE_CENTRAL_URL, so an agent inside a sandbox
or container reaches the host by setting that variable; the repository
never changes. The code-central CLI inside it reads the same variable.

Docker Sandboxes (sbx)
  Let the sandbox's proxy through to the host's port once:
    sbx policy allow network localhost:7733

Plain Linux Docker
  A container can't reach the host's loopback. Add the Docker bridge
  address to the service's listeners, then point the container at it:
    code-central serve --bind 172.17.0.1
    docker run -e CODE_CENTRAL_URL=http://172.17.0.1:7733 ...
  --bind adds one specific address. Never give it a LAN address: the
  service has no authentication, and it never binds 0.0.0.0.

Docker Desktop
  Try CODE_CENTRAL_URL=http://host.docker.internal:7733 in the container;
  if it can't connect, use --bind as for plain Linux Docker.
`,
}

func runHelpTopic(e Env, topic string) int {
	text, ok := helpTopics[topic]
	if !ok {
		names := make([]string, 0, len(helpTopics))
		for n := range helpTopics {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(e.Stderr, "code-central help: no topic %q; topics: %s\n", topic, strings.Join(names, ", "))
		return exitUsage
	}
	fmt.Fprint(e.Stdout, text)
	return exitOK
}
