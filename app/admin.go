package app

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/rs/zerolog"
)

func (c *App) MatrixAdminProxy() http.HandlerFunc {

	endpoint := fmt.Sprintf("%s/_synapse/", c.Config.Matrix.Homeserver)
	target, _ := url.Parse(endpoint)

	proxy := httputil.NewSingleHostReverseProxy(target)

	return func(w http.ResponseWriter, r *http.Request) {

		user_id := c.AuthenticatedUser(r)
		access_token := c.AuthenticatedAccessToken(r)

		c.Log.Info().
			Dict("details", zerolog.Dict().
				Str("user", *user_id).
				Str("access_token", *access_token).
				Str("api", fmt.Sprintf("%s %s", r.Method, r.URL.Path)),
			).Msg("Accessing admin API")

		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *access_token))
		w.Header().Del("Access-Control-Allow-Origin")

		proxy.ServeHTTP(w, r)
	}

}
