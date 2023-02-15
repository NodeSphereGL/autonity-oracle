package httpserver

import (
	"autonity-oracle/oracle_server"
	"autonity-oracle/types"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-hclog"
	"github.com/modern-go/reflect2"
	"net/http"
	o "os"
)

type HTTPServer struct {
	logger hclog.Logger
	http.Server
	oracle *oracleserver.OracleServer
	port   int
}

func NewHttpServer(os *oracleserver.OracleServer, port int) *HTTPServer {
	hs := &HTTPServer{
		oracle: os,
		port:   port,
	}
	router := hs.createRouter()
	hs.logger = hclog.New(&hclog.LoggerOptions{
		Name:   reflect2.TypeOfPtr(hs).String(),
		Output: o.Stdout,
		Level:  hclog.Debug,
	})
	hs.Addr = fmt.Sprintf(":%d", port)
	hs.Handler = router
	return hs
}

// StartHTTPServer start the http server in a new go routine.
func (hs *HTTPServer) StartHTTPServer() {
	go func() {
		if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Error("HTTP service listen on port: ", hs.port, err)
			panic(err.Error())
		}
	}()
}

func (hs *HTTPServer) createRouter() *gin.Engine {
	// create http api handlers.
	gin.SetMode("release")
	router := gin.Default()
	router.POST("/", func(c *gin.Context) {
		var reqMsg types.JSONRPCMessage
		if err := json.NewDecoder(c.Request.Body).Decode(&reqMsg); err != nil {
			c.JSON(http.StatusBadRequest, types.JSONRPCMessage{Error: err.Error()})
		}
		hs.logger.Debug("handling method:", reqMsg.Method)
		switch reqMsg.Method {
		case "list_plugins":
			c.JSON(hs.listPlugins(&reqMsg))
		case "get_version":
			c.JSON(hs.getVersion(&reqMsg))
		case "get_prices":
			c.JSON(hs.getPrices(&reqMsg))
		default:
			c.JSON(http.StatusBadRequest, types.JSONRPCMessage{ID: reqMsg.ID, Error: "unknown method"})
		}
	})
	return router
}

// handler functions
func (hs *HTTPServer) getVersion(reqMsg *types.JSONRPCMessage) (int, types.JSONRPCMessage) {
	type Version struct {
		Version string
	}

	enc, err := json.Marshal(Version{Version: hs.oracle.Version()})
	if err != nil {
		return http.StatusInternalServerError, types.JSONRPCMessage{Error: err.Error()}
	}

	return http.StatusOK, types.JSONRPCMessage{ID: reqMsg.ID, Result: enc}
}

func (hs *HTTPServer) getPrices(reqMsg *types.JSONRPCMessage) (int, types.JSONRPCMessage) {
	type PriceAndSymbol struct {
		Prices  types.PriceBySymbol
		Symbols []string
	}
	enc, err := json.Marshal(PriceAndSymbol{
		Prices:  hs.oracle.GetPrices(),
		Symbols: hs.oracle.Symbols(),
	})
	if err != nil {
		return http.StatusInternalServerError, types.JSONRPCMessage{Error: err.Error()}
	}
	return http.StatusOK, types.JSONRPCMessage{ID: reqMsg.ID, Result: enc}
}

func (hs *HTTPServer) listPlugins(reqMsg *types.JSONRPCMessage) (int, types.JSONRPCMessage) {
	enc, err := json.Marshal(hs.oracle.GetPlugins())
	if err != nil {
		return http.StatusInternalServerError, types.JSONRPCMessage{Error: err.Error()}
	}
	return http.StatusOK, types.JSONRPCMessage{ID: reqMsg.ID, Result: enc}
}
