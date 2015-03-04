package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/yleemj/dockerMan/app/cluster"
	"github.com/yleemj/dockerMan/app/manager"
)

var (
	listenAddr        string
	mongodbAddr       string
	mongodbDatabase   string
	disableUsageInfo  bool
	showVersion       bool
	controllerManager *manager.Manager
	logger            = logrus.New()
)

const (
	STORE_KEY = "dockerMan"
	VERSION   = "0.0.1"
)

func init() {
	flag.StringVar(&listenAddr, "listen", ":8080", "listen address")
	flag.StringVar(&mongodbAddr, "mongodb-addr", "127.0.0.1:27017", "mongodb address")
	flag.StringVar(&mongodbDatabase, "mongodb-database", "dockerMan", "mongodb database")
	flag.BoolVar(&disableUsageInfo, "disable-usage-info", false, "disable anonymous usage info")
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
}

func destroy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	container, err := controllerManager.Container(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := controllerManager.Destroy(container); err != nil {
		logger.Errorf("error destroying %s: %s", container.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Infof("destroyed container %s (%s)", container.ID, container.Image.Name)

	w.WriteHeader(http.StatusNoContent)
}

func run(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	p := r.FormValue("pull")
	c := r.FormValue("count")
	count := 1
	pull := false
	if p != "" {
		pv, err := strconv.ParseBool(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		pull = pv
	}
	if c != "" {
		cc, err := strconv.Atoi(c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		count = cc
	}
	var image *cluster.Image
	if err := json.NewDecoder(r.Body).Decode(&image); err != nil {
		logger.Warnf("error decoding image: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	launched, err := controllerManager.Run(image, count, pull)
	if err != nil {
		logger.Warnf("error running container: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusCreated)

	if err := json.NewEncoder(w).Encode(launched); err != nil {
		logger.Error(err)
	}
}

func stopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	container, err := controllerManager.Container(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := controllerManager.ClusterManager().Stop(container); err != nil {
		logger.Errorf("error stopping %s: %s", container.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Infof("stopped container %s (%s)", container.ID, container.Image.Name)

	w.WriteHeader(http.StatusNoContent)
}

func restartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	container, err := controllerManager.Container(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := controllerManager.ClusterManager().Restart(container, 10); err != nil {
		logger.Errorf("error restarting %s: %s", container.ID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Infof("restarted container %s (%s)", container.ID, container.Image.Name)

	w.WriteHeader(http.StatusNoContent)
}

func engines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	engines := controllerManager.Engines()
	if err := json.NewEncoder(w).Encode(engines); err != nil {
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func inspectEngine(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]
	engine := controllerManager.Engine(id)
	if err := json.NewEncoder(w).Encode(engine); err != nil {
		logger.Error(err)
	}
}

func containers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	containers := controllerManager.Containers(true)
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		logger.Error(err)
	}
}

func inspectContainer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)
	id := vars["id"]
	container, err := controllerManager.Container(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(container); err != nil {
		logger.Error(err)
	}
}

func clusterInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	info := controllerManager.ClusterInfo()
	if err := json.NewEncoder(w).Encode(info); err != nil {
		logger.Error(err)
	}
}

func main() {
	mHost := os.Getenv("MONGO_PORT_27017_TCP_ADDR")
	mPort := os.Getenv("MONGO_PORT_27017_TCP_PORT")
	mDb := os.Getenv("MONGO_DATABASE")

	if mHost != "" && mPort != "" {
		mongodbAddr = fmt.Sprintf("%s:%s", mHost, mPort)
	}
	if mDb != "" {
		mongodbDatabase = mDb
	}

	flag.Parse()
	if showVersion {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	var (
		mErr      error
		globalMux = http.NewServeMux()
	)

	logger.Infof("dockerMan version %s", VERSION)

	controllerManager, mErr = manager.NewManager(mongodbAddr, mongodbDatabase, VERSION, disableUsageInfo)
	if mErr != nil {
		logger.Fatal(mErr)
	}

	apiRouter := mux.NewRouter()
	apiRouter.HandleFunc("/api/cluster/info", clusterInfo).Methods("GET")
	apiRouter.HandleFunc("/api/containers", containers).Methods("GET")
	apiRouter.HandleFunc("/api/containers", run).Methods("POST")
	apiRouter.HandleFunc("/api/containers/{id}", inspectContainer).Methods("GET")
	apiRouter.HandleFunc("/api/containers/{id}", destroy).Methods("DELETE")
	apiRouter.HandleFunc("/api/containers/{id}/stop", stopContainer).Methods("GET")
	apiRouter.HandleFunc("/api/containers/{id}/restart", restartContainer).Methods("GET")
	apiRouter.HandleFunc("/api/engines", engines).Methods("GET")
	apiRouter.HandleFunc("/api/engines/{id}", inspectEngine).Methods("GET")

	// global handler
	globalMux.Handle("/", http.FileServer(http.Dir("static")))

	// api router; protected by auth
	apiAuthRouter := negroni.New()
	//apiAuthRequired := auth.NewAuthRequired(controllerManager)
	//apiAccessRequired := access.NewAccessRequired(controllerManager)
	//apiAuthRouter.Use(negroni.HandlerFunc(apiAuthRequired.HandlerFuncWithNext))
	//apiAuthRouter.Use(negroni.HandlerFunc(apiAccessRequired.HandlerFuncWithNext))
	apiAuthRouter.UseHandler(apiRouter)
	globalMux.Handle("/api/", apiAuthRouter)

	if err := http.ListenAndServe(listenAddr, context.ClearHandler(globalMux)); err != nil {
		logger.Fatal(err)
	}
}
