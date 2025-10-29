# Setup local Kubernetes

## Docker Desktop

Installing local Kubernetes cluster with docker desktop is the preferred method in this tutorial
as it is very easy and comes with kubectl CLI.

If you have not installed kubernetes from docker-desktop before:

To enable Kubernetes in Docker Desktop:
- 1: Open the GUI
- 2: Click on the gear icon in the upper right corner
- 3: Click on the "Enable Kubernetes" toggle. Just leave everything as default.
- 4: Click "Apply & Restart" in the lower right corner. 
- 5: Wait until it says "Running" under "Cluster"
- 6: Open a new terminal and check that you are able to run ``kubectl version``

Expected output for kubectl version is 
```plaintext
Client Version: v<your-version>
Kustomize Version: v<your-version>
Server Version: v<your-version>
```

If you have kubectl previously installed it is nice to 
verify that you are doing this tutorial in docker desktop and not on your 
production cluster with 

```bash
kubectl config use-context docker-desktop
```

## Build docker images

Go to `app/frontend` directory
To build docker image run

```bash
docker build -t blog-frontend:0.1 .
```

Go to `app/api` directory.
Build docker image with

```bash
docker build -t blog-api:0.1 .
```

Go to `app/image-job`
```bash
docker build -t image-job:0.1 .
```

**If you do not have problems with the above setup, you do not have to read the rest**

## Other options (not tested for this tutorial)

### kind
https://kind.sigs.k8s.io/docs/user/quick-start/

### minikube
https://minikube.sigs.k8s.io/docs/start/

### k3d
https://k3d.io/stable/


## Debug: Install kubectl manually

If you cant run kubectl after enabling Kubernetes in Docker Desktop, you can try installing it manually
through:
https://kubernetes.io/docs/tasks/tools/#kubectl

It is also available through homebrew (mac):
brew install kubectl


[Next task](../task-1-run-frontend-pod/)
