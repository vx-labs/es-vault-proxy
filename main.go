package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/spf13/cobra"
)

func loadVaultToken(api *api.Client) {
	fallback := func() {
		api.SetToken(os.Getenv("VAULT_TOKEN"))
	}
	_, err := os.Stat("secrets/vault_token")
	if err != nil {
		fallback()
		return
	}
	token, err := ioutil.ReadFile("secrets/vault_token")
	if err != nil {
		fallback()
		return
	}
	api.SetToken(string(token))
	go func() {
		sigUsr1 := make(chan os.Signal, 1)
		signal.Notify(sigUsr1, syscall.SIGUSR1)
		<-sigUsr1
		log.Println("INFO: received SIGUSR1, reloading vault token")
		loadVaultToken(api)
	}()
}

func getString(cmd *cobra.Command, key string) string {
	value, err := cmd.Flags().GetString(key)
	if err != nil {
		log.Fatalf("failed to read %s flag: %v", key, err)
	}
	return value
}

type ESConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func getConfig(vault *api.Client, configKey string) (ESConfig, error) {
	out := struct {
		Data ESConfig `json:"data"`
	}{}
	req := vault.NewRequest("GET", fmt.Sprintf("/v1%s", configKey))
	resp, err := vault.RawRequest(req)
	if err != nil {
		return out.Data, err
	}
	defer resp.Body.Close()
	return out.Data, resp.DecodeJSON(&out)

}
func run(vault *api.Client, configKey string) error {
	httpc := http.DefaultClient
	httpc.Timeout = 5 * time.Second
	httpc.Transport = http.DefaultTransport
	if proxy := os.Getenv("HTTPS_PROXY"); proxy != "" {
		log.Printf("proxy: %s", proxy)
	}
	listen := "[::]:9200"

	log.Printf("ES proxy is listening on %s", listen)
	return http.ListenAndServe(listen, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			log.Printf("incoming: %s %s", r.Method, r.URL.String())
			config, err := getConfig(vault, configKey)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err)
				return
			}
			urlString := fmt.Sprintf("%s%s", config.URL, r.URL.Path)
			query := r.URL.Query().Encode()
			if query != "" {
				urlString = fmt.Sprintf("%s?%s", urlString, query)
			}
			egressReq, err := http.NewRequest(r.Method, urlString, r.Body)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err)
				return
			}
			for name, values := range r.Header {
				for _, value := range values {
					egressReq.Header.Add(name, value)
				}
			}
			egressReq.SetBasicAuth(config.Username, config.Password)
			resp, err := httpc.Do(egressReq)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err)
				return
			}

			log.Printf("outgoing: %s %s %d", egressReq.Method, egressReq.URL.String(), resp.StatusCode)
			for name, values := range resp.Header {
				for _, value := range values {
					if resp.StatusCode > 399 {
						log.Printf("  %s: %s", name, value)
					}
					w.Header().Add(name, value)
				}
			}
			w.WriteHeader(resp.StatusCode)
			if resp.Body != nil {
				io.Copy(w, resp.Body)
				resp.Body.Close()
			}

		}),
	)
}

func main() {
	cmd := cobra.Command{
		Use: "es-vault-proxy",
		Run: func(cmd *cobra.Command, args []string) {
			config := api.DefaultConfig()
			vault, err := api.NewClient(config)
			configKey := getString(cmd, "config-key")
			if err != nil {
				log.Fatalf("failed to create vault client: %v", err)
			}
			loadVaultToken(vault)
			err = run(vault, configKey)
			if err != nil {
				log.Println(err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringP("config-key", "c", "/secret/data/tracing/es", "configuration key in Vault")
	cmd.Execute()
}
