//go:build windows && amd64

package httptransport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/abworks-dev/aboard-host/internal/workspace"
)

func TestHTTPRejectedStalledBodiesCloseWithoutImplicitDrain(t *testing.T) {
	f := newHTTPFixture(t)
	a := ownerAuthorization(t, f)
	for _, shape := range []string{"Content-Length: 1", "Transfer-Encoding: chunked"} {
		for _, kind := range []string{"authorization", "route", "capacity"} {
			t.Run(kind+shape, func(t *testing.T) {
				var refs []*workspace.OperationReservation
				if kind == "capacity" {
					refs = fillOrdinary(t, f, a, workspace.InstallationOperationLimit-1)
					defer func() {
						for _, r := range refs {
							r.Release()
						}
					}()
				}
				path := "/v1/context"
				if kind == "route" {
					path = "/v1/not-an-operation"
				}
				auth := "Authorization: Bearer " + f.token + "\r\n"
				if kind == "authorization" {
					auth = ""
				}
				c, err := net.DialTimeout("tcp", f.handler.authority, 2*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(2 * time.Second))
				_, err = fmt.Fprintf(c, "GET %s HTTP/1.1\r\nHost: %s\r\n%s%s\r\n\r\n", path, f.handler.authority, auth, shape)
				if err != nil {
					t.Fatal(err)
				}
				reader := bufio.NewReader(c)
				response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
				if err != nil {
					t.Fatal("rejection waited for unsent body", err)
				}
				if response.StatusCode < 400 {
					t.Fatal("not rejected", response.StatusCode)
				}
				if _, err = io.ReadAll(response.Body); err != nil {
					t.Fatal("response stalled", err)
				}
				response.Body.Close()
				if !response.Close {
					t.Fatal("unread request connection remained reusable")
				}
				if _, err = reader.ReadByte(); err != io.EOF {
					t.Fatal("transport failed to finish without client body", err)
				}
			})
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := f.handler.Pause(ctx); err != nil {
		t.Fatal("rejections left active body readers", err)
	}
}
