SHELL := /bin/sh

HOST ?= 127.0.0.1
PORT ?= 8080
DEMO_DIR ?= .demo
DEMO_BIN ?= $(DEMO_DIR)/neul-server
DEMO_DB ?= $(DEMO_DIR)/neul.sqlite
DEMO_LOG ?= $(DEMO_DIR)/neul-server.log
DEMO_PID ?= $(DEMO_DIR)/neul-server.pid
DEMO_ADDR_FILE ?= $(DEMO_DIR)/neul-server.addr
DEMO_HOME ?= $(DEMO_DIR)/home
DEMO_STATIC_DIR ?= web/dist
DEMO_ENV = HOST="$(HOST)" PORT="$(PORT)" DEMO_DIR="$(DEMO_DIR)" DEMO_BIN="$(DEMO_BIN)" DEMO_DB="$(DEMO_DB)" DEMO_LOG="$(DEMO_LOG)" DEMO_PID="$(DEMO_PID)" DEMO_ADDR_FILE="$(DEMO_ADDR_FILE)" DEMO_HOME="$(DEMO_HOME)" DEMO_STATIC_DIR="$(DEMO_STATIC_DIR)"

.PHONY: demo demo-stop demo-clean demo-status verify-docs verify-demo

demo:
	@$(DEMO_ENV) sh scripts/demo.sh start

demo-stop:
	@$(DEMO_ENV) sh scripts/demo.sh stop

demo-clean:
	@$(DEMO_ENV) sh scripts/demo.sh clean

demo-status:
	@$(DEMO_ENV) sh scripts/demo.sh status

verify-docs:
	@sh scripts/validate-demo-docs.sh
	@sh scripts/validate-packaged-client-docs.sh

verify-demo:
	@sh scripts/verify-demo-pid-safety.sh
	@sh scripts/verify-demo-smoke.sh
