package api

import (
	"net/http"

	"chainspace.io/blockmania/internal/log"
	"chainspace.io/blockmania/internal/log/fld"
	_ "chainspace.io/blockmania/rest/api/docs"
	"chainspace.io/blockmania/rest/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
)

type Router struct {
	*gin.Engine
	srv        service.Service
	wssrv      service.WSService
	wsupgrader WSUpgrader
}

// @title blockmania API
// @version 1.0
// @description blockmania endpoints

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

func New(srv service.Service, wssrv service.WSService) *Router {
	r := Router{
		Engine:     gin.Default(),
		srv:        srv,
		wssrv:      wssrv,
		wsupgrader: upgrader{},
	}
	r.Use(cors.Default())

	// swagger docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// endpoints
	r.POST("api/transaction", r.addTransaction)
	r.POST("api/transactions", r.addTransactions)
	r.GET("/api/pubsub/ws", r.websocket)

	// healthcheck
	r.GET("/healthcheck", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return &r
}

// addTransaction
// @Summary Add a new transaction in the blockchain
// @Description Add a new transaction in the blockchain
// @ID addTransaction
// @Accept  json
// @Produce  json
// @Tags blockmania
// @Param   transaction      body   api.TransactionRequest     true  "transaction"
// @Success 200 {object} api.Response
// @Success 400 {object} api.Response
// @Success 500 {object} api.Response
// @Router /api/transaction [post]
func (r *Router) addTransaction(c *gin.Context) {
	req := TransactionRequest{}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Error: err.Error()})
		return
	}
	err := r.srv.AddTransaction(req.Tx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{})
}

// addTransactions
// @Summary Add a list of new transactions in the blockchain
// @Description Add a list of new transactions in the blockchain
// @ID addTransactions
// @Accept  json
// @Produce  json
// @Tags blockmania
// @Param   transactions      body   api.TransactionsRequest     true  "transactions"
// @Success 200 {object} api.Response
// @Success 400 {object} api.Response
// @Success 500 {object} api.Response
// @Router /api/transactions [post]
func (r *Router) addTransactions(c *gin.Context) {
	req := TransactionsRequest{}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Error: err.Error()})
		return
	}
	err := r.srv.AddTransactions(req.Txs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{})

}

// websocket Initiate a websocket connection
// @Summary Initiate a websocket connection in order to subscribe to objects saved in chainspace by the current node
// @Description Initiate a websocket connection
// @ID websocket
// @Tags pubsub
// @Success 204
// @Failure 500 {object} api.Response
// @Router /api/pubsub/ws [get]
func (r *Router) websocket(c *gin.Context) {
	wc, err := r.wsupgrader.Upgrade(c.Writer, c.Request)
	if err != nil {
		log.Error("unable to upgrade ws connection", fld.Err(err))
		c.JSON(http.StatusInternalServerError, Response{err.Error()})
		return
	}

	if status, err := r.wssrv.Websocket(c.Request.RemoteAddr, wc); err != nil {
		log.Error("unexpected closed websocket connection", fld.Err(err))
		c.JSON(status, Response{err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
