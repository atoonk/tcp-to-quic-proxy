package cmd

import (
	"fmt"
	"os"

	"github.com/atoonk/tcp-to-any-proxy/internal/proxy"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Run: proxy.RunProxy,
}

func init() {
	rootCmd.PersistentFlags().StringP("localAddr", "l", "localhost:9999", "local address")
	rootCmd.PersistentFlags().StringP("remoteAddr", "r", "127.0.0.1:5201", "remote address")
	rootCmd.PersistentFlags().StringP("listenProto", "p", "tcp", "listen protocol, either tcp, quic or ws")
	rootCmd.PersistentFlags().StringP("remoteProto", "u", "tcp", "remote protocol either tcp, quic or ws")

}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
