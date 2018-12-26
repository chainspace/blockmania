package api

import (
	"net/http"

	_ "chainspace.io/blockmania/rest/api/docs"
	"chainspace.io/blockmania/rest/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
)

type Router struct {
	*gin.Engine
	srv service.Service
}

// @title blockmania API
// @version 1.0
// @description blockmania endpoints
// @termsOfService http://swagger.io/terms/

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

func New(srv service.Service) *Router {
	r := Router{
		Engine: gin.Default(),
		srv:    srv,
	}
	r.Use(cors.Default())

	// swagger docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// endpoints
	r.POST("api/transaction", r.addTransaction)
	r.POST("api/transactions", r.addTransactions)

	// healthcheck
	r.GET("/healthcheck", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return &r
}

// addTransaction
// @Summary Add a new transaction in the blockchain
// @Description Add a new transction in the blockchain
// @ID addTransction
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
// @Description Add a list of new transctions in the blockchain
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
