
# Task 7 - API security context

In this task we will enforce and provide a security context for the API pods.

First remove the api pods from the cluster with

```bash
kubectl delete -f api.yaml
```

We will create a rule on the whole `new-ns` that all pods must have certain security features to be allowed to by putting the following label on the namespace

```plaintext
pod-security.kubernetes.io/enforce: restricted
```

so the whole new-ns.yaml should look like this

```yaml
apiVersion: v1
kind: Namespace
metadata:
  creationTimestamp: null
  name: new-ns
  labels:
    pod-security.kubernetes.io/enforce: restricted
spec: {}
status: {}
```

Now apply that with 
```bash
kubectl apply -f new-ns.yaml
```

We can now apply the api.yaml again with
```bash
kubectl apply -f api.yaml
```

Now we get a warning

```plaintext
Warning: would violate PodSecurity "restricted:latest": allowPrivilegeEscalation != false (container "my-blog-api" must set securityContext.allowPrivilegeEscalation=false), unrestricted capabilities (container "my-blog-api" must set securityContext.capabilities.drop=["ALL"]), runAsNonRoot != true (pod or container "my-blog-api" must set securityContext.runAsNonRoot=true), seccompProfile (pod or container "my-blog-api" must set securityContext.seccompProfile.type to "RuntimeDefault" or "Localhost")
deployment.apps/my-api created
```

and we see that we have no pods in the `new-ns` with

```bash
kubectl get pods -n new-ns
```

To fulfill the new security requirements, we provide what was mentioned in the warning in a new `securityContext` field.

```yaml
      containers:
      - image: blog-api:0.1
        name: my-blog-api
        resources: {}
        env:
          - name: REDIS_ADDR
            value: redis.default.svc.cluster.local:6379
        securityContext:
          runAsNonRoot: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          seccompProfile:
            type: RuntimeDefault
```

and apply it again

```bash
kubectl apply -f api.yaml
```


Now we get no warning, but if we do 
```bash
kubectl get pods -n new-ns 
```

we see that it has STATUS CreateContainerConfigError.

We can get more information about this error by running

```bash
kubectl describe pod -n new-ns my-api- # press tab to autocomplete
```

In the bottom at the events section we see

```bash
container has runAsNonRoot and image has non-numeric user
```

this can be fixed by also adding a `runAsUser` field in the securityContext with a number different than 0 (root user)

Update the securityContext so it runs as a non-root user with

```yaml
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          seccompProfile:
            type: RuntimeDefault
```

Now we can check if the pods run again with

```bash
kubectl get pods -n new-ns
```

and they should be running and the app working 


## Verify solution

From the workdir folder run

```bash
git diff --no-index . ../tasks/task-7-api-security-context/solution
```

and check that there is no diff


[Next task](../task-8-wrapping-up/)
