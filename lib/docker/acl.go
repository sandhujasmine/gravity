package docker

import (
	"fmt"
	"net/http"

	"github.com/gravitational/gravity/lib/httplib"
	"github.com/gravitational/gravity/lib/users"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

type registryACL struct {
	Users users.Identity
}

func newACL(parameters map[string]interface{}) (auth.AccessController, error) {
	usersI, ok := parameters["users"]
	if !ok {
		return nil, trace.BadParameter("missing Users")
	}
	users, ok := usersI.(users.Identity)
	if !ok {
		return nil, trace.BadParameter("expected users.Identity, got: %T", usersI)
	}
	return &registryACL{
		Users: users,
	}, nil
}

func (acl *registryACL) Authorized(ctx context.Context, accessItems ...auth.Access) (context.Context, error) {
	request, err := context.GetRequest(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	authCreds, err := httplib.ParseAuthHeaders(request)
	if err != nil {
		return nil, &challenge{
			realm: "basic-realm",
			err:   auth.ErrInvalidCredential,
		}
	}
	user, _, err := acl.Users.AuthenticateUser(*authCreds)
	if err != nil {
		logrus.Warnf("Authentication failure for %v: %v.", authCreds.Username, err)
		return nil, &challenge{
			realm: "basic-realm",
			err:   auth.ErrAuthenticationFailure,
		}
	}
	return auth.WithUser(ctx, auth.UserInfo{
		Name: user.GetName(),
	}), nil
}

type challenge struct {
	realm string
	err   error
}

var _ auth.Challenge = challenge{}

func (ch challenge) SetHeaders(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", ch.realm))
}

func (ch challenge) Error() string {
	return fmt.Sprintf("basic authentication challenge for realm %q: %s", ch.realm, ch.err)
}

func init() {
	auth.Register("gravity", auth.InitFunc(newACL))
}
