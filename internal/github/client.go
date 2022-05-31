package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// NewClient returns a new GitHub API client. This function abstracts the
// onerous process of authenticating as an installation. It uses the provided
// appID and ASCII-armored x509 certificate key to create a JWT (JSON web token)
// that is used to authenticate to the GitHub Apps API. Using that API, an
// installation token is obtained for the given installationID. This token is
// ultimately used by the returned client to authenticate as the specified
// installation.
//
// See the following for further details:
// https://docs.github.com/en/developers/apps/authenticating-with-github-apps
func NewClient(
	ctx context.Context,
	appID int64,
	installationID int64,
	keyPEM []byte,
) (*github.Client, error) {
	installationToken, err := getInstallationToken(
		ctx,
		appID,
		installationID,
		keyPEM,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to negotiate an installation token: %s", err)
	}
	return github.NewClient(
		oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(
				&oauth2.Token{
					TokenType:   "token", // This type indicates an installation token
					AccessToken: installationToken,
				},
			),
		),
	), nil
}

// getInstallationToken returns an installation token for the given appID and
// installationID. It uses the provided appID and ASCII-armored x509 certificate
// key to create a JWT that is used to authenticate to the GitHub Apps API.
// Using that API, an installation token is obtained for the given
// installationID.
func getInstallationToken(
	ctx context.Context,
	appID int64,
	installationID int64,
	keyPEM []byte,
) (string, error) {
	jwt, err := createJWT(appID, keyPEM)
	if err != nil {
		return "", errors.Wrapf(
			err,
			"error getting signed JSON web token for installation %d",
			installationID,
		)
	}
	appsClient := newAppsClientFromJWT(ctx, jwt)
	installationToken, _, err := appsClient.CreateInstallationToken(
		ctx,
		installationID,
		&github.InstallationTokenOptions{},
	)
	if err != nil {
		return "", errors.Wrapf(
			err,
			"error creating installation token for installation %d",
			installationID,
		)
	}
	return installationToken.GetToken(), nil
}

// createJWT uses the provided appID and ASCII-armored x509 certificate key to
// create a JWT that can be used to authenticate to GitHub APIs as the specified
// App.
func createJWT(appID int64, keyPEM []byte) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyPEM)
	if err != nil {
		return "", err
	}
	now := time.Now()
	return jwt.NewWithClaims(
		jwt.SigningMethodRS256,
		jwt.StandardClaims{
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(5 * time.Minute).Unix(),
			Issuer:    strconv.FormatInt(appID, 10),
		},
	).SignedString(key)
}

// newAppsClientFromJWT returns a new client for the GitHub Apps API that will
// authenticate as an App using the provided JWT.
func newAppsClientFromJWT(ctx context.Context, jwt string) *github.AppsService {
	return github.NewClient(
		oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(
				&oauth2.Token{
					AccessToken: jwt,
				},
			),
		),
	).Apps
}
