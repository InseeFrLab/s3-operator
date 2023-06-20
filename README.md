# s3-operator

This Operator SDK based tool aims at managing S3 related resources (buckets, policies, ...) using a Kubernetes-centric approach. You can set `Bucket` or `Policy` custom resources, and let the operator create or update the corresponding bucket/policy on its configured S3 instance.

## At a glance

- Current S3 providers : [Minio](https://github.com/InseeFrLab/s3-operator/blob/main/controllers/s3/factory/minioS3Client.go) (a [mock](https://github.com/InseeFrLab/s3-operator/blob/main/controllers/s3/factory/mockedS3Client.go) implementation is also present for testing purposes)
- Currently managed S3 resources : [buckets](https://github.com/InseeFrLab/s3-operator/blob/main/api/v1alpha1/bucket_types.go), [policies](https://github.com/InseeFrLab/s3-operator/blob/main/api/v1alpha1/policy_types.go)

## Compatibility

So far, this operator has been tested with : 

- Kubernetes : 1.25, 1.26
- MinIO : 2023-05-27T05:56:19Z

## Description

At its heart, the operator revolves around CRDs that match S3 resources : 

- `buckets.s3.onyxia.sh`
- `policies.s3.onyxia.sh`

The custom resources based on these CRDs are a somewhat simplified projection of the real S3 resources. From the operator's point of view : 

- A `Bucket` CR only has a name, a quota (actually two, more on this below), and optionally, a set of paths
- A `Policy` CR has a name, and its actual content (IAM JSON)

Each custom resource based on these CRDs on Kubernetes is to be matched with a resource on the S3 instance. If the CR and the corresponding S3 resource diverge, the operator will create or update the S3 resource to bring it back to .

Two important caveats : 

- It is one-way - if something happens on the S3 side directly (instead of going through the CRs), the operator has no way of reacting. At best, the next trigger will overwrite the S3 state with the declared state in the k8s custom resource.
- For now, the operator won't delete any resource on S3 - if a CR is removed, its matching resource on S3 will still be present. This behavior was primarily picked to avoid data loss for bucket, but also applied to policies - which could be debatable.

---
```
NB : the remainder of this README comes from the auto-generated Operator SDK placeholder. As it remains a useful reference, it is left as is until it's replaced by a documentation more accurately focused on this operator.
```
---


## Getting Started

You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/s3-operator:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/s3-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
