# Task 01 - Run Frontend Pod


## Create and access pod imperatively

We will run our first pod with

```bash
kubectl run my-frontend --image blog-frontend:0.1
```

To check that it is running we can do

```bash
kubectl get pods
```

We should see that it has STATUS Running

We can also check the logs of the running pod with

```bash
kubectl logs my-frontend
```

To temporarily expose the frontend on localhost we can run the following command

```bash
kubectl port-forward pod/my-frontend 8045:8045
```

and let it run. Then it is possible to access the frontend on
http://localhost:8045

```bash
kubectl delete pod  my-frontend
```

## Create and access pod declarative way

We can use the same command as in the first section, but append --dry-run=client and -o yaml
to get a yaml template, which is what kubernetes uses as IaC.

In the following command we also pipe the output into a file:

```bash
kubectl run my-frontend --image blog-frontend:0.1 --dry-run=client -o yaml > frontend-pod.yaml
```

With the above command, the pod does not get scheduled to the cluster, and we need to apply the yaml definition to get the pod in the cluster with

```bash
kubectl apply -f frontend-pod.yaml
```

To check that it is running we can again do 

```bash
kubectl get pods
```

We can delete the pod again with

```bash
kubectl delete -f frontend-pod.yaml
```

Check that there is no pod running with

```bash
kubectl get pods
```

and delete the frontend-pod.yaml

```bash
rm frontend-pod.yaml
```


[Next task](../task-2-run-frontend-deployment/)
