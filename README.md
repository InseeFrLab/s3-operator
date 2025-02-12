# s3-operator

This Operator SDK based tool aims at managing S3 related resources (buckets, policies, ...) using a Kubernetes-centric approach. You can set `Bucket` or `Policy` custom resources, and let the operator create or update the corresponding bucket/policy on its configured S3 instance.

## At a glance

- Current S3 providers : [Minio](https://github.com/InseeFrLab/s3-operator/blob/main/internal/s3/factory/minioS3Client.go)
- Currently managed S3 resources : [buckets](https://github.com/InseeFrLab/s3-operator/blob/main/api/v1alpha1/bucket_types.go), [policies](https://github.com/InseeFrLab/s3-operator/blob/main/api/v1alpha1/policy_types.go)

## Compatibility

This operator has been successfully tested with : 

- Kubernetes : 1.25, 1.26, 1.27 (v0.7.0), 1.28 (v0.7.0 and v0.8.0)
- MinIO : 2023-05-27T05:56:19Z (up to v0.3.0 included), 2023-11-20T22-40-07Z (from v0.4.0 onwards)

## Description

At its heart, the operator revolves around CRDs that match S3 resources : 

- `buckets.s3.onyxia.sh`
- `policies.s3.onyxia.sh`
- `paths.s3.onyxia.sh`
- `s3Users.s3.onyxia.sh`
- `s3Instances.s3.onyxia.sh`

The custom resources based on these CRDs are a somewhat simplified projection of the real S3 resources. From the operator's point of view : 

- A `Bucket` CR matches a S3 bucket, and only has a name, a quota (actually two, [see Bucket example in *Usage* section below](#bucket)), and optionally, a set of paths
- A `Policy` CR matches a "canned" policy (not a bucket policy, but a global one, that can be attached to a user), and has a name, and its actual content (IAM JSON)
- A `Path` CR matches a set of paths inside of a policy. This is akin to the `paths` property of the `Bucket` CRD, except `Path` is not responsible for Bucket creation. 
- A `S3User` CR matches a user in the s3 server, and has a name, a set of policy and a set of group.
- A `S3Instance` CR matches a s3Instance. 

Each custom resource based on these CRDs on Kubernetes is to be matched with a resource on the S3 instance. If the CR and the corresponding S3 resource diverge, the operator will create or update the S3 resource to bring it back to.

Two important caveats : 

- It is one-way - if something happens on the S3 side directly (instead of going through the CRs), the operator has no way of reacting. At best, the next trigger will overwrite the S3 state with the declared state in the k8s custom resource.
- Originally, the operator did not manage resource deletion. This has changed in release v0.8.0 (see #40), but it still isn't a focus, and the implementation is simple. For instance, bucket deletion will simply fail if bucket is not empty - no logic was added to opt-in a "forced" deletion of everything inside the bucket.

## Installation

The S3 operator is provided either in source form through this repository, or already built as a Docker image available on [Docker Hub](https://hub.docker.com/r/inseefrlab/s3-operator).

### Helm

With this Docker image, the recommended way of installing S3 Operator on your cluster is to use the Helm chart provided in the dedicated repository : https://github.com/InseeFrLab/helm-charts/tree/master/charts/s3-operator. Among other things, the chart takes care of managing a (Kubernetes) ServiceAccount for the operator to run as. The most basic way of using this chart would be : 

```shell
helm repo add inseefrlab https://inseefrlab.github.io/helm-charts # or [helm repo update] if already available
helm install <name> s3-operator --values <yaml-file/url>  # see below for the parameters
```

### Running from source

Alternatively, if you just wish to try out the operator without actually installing it, it is also possible to just clone this repository, and run the operator locally - outside of the Kubernetes cluster. This requires Go 1.19+, and prior installation of the CRDs located in `config/crd/bases`, typically with `kubectl`. After which, you can simply run : 

```shell
git clone https://github.com/InseeFrLab/s3-operator.git # or use a tag/release
cd s3-operator
go run main.go --s3-endpoint-url *** --s3-access-key *** --s3-secret-key *** # see below for the parameters
```

To quote the Operator SDK README (also visible below), running the operator this way *will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).* RBAC-wise, you need to be able to freely manipulate the custom resources associated to the operator (`Bucket`, `Policy` and `Path`) in every namespace - [see also the generated ClusterRole manifest](https://github.com/InseeFrLab/s3-operator/blob/main/config/rbac/role.yaml).

### Kustomize

Finally, as this operator was generated through Operator SDK, it should be possible to use kustomize to bootstrap the operator as well. Though this method is untested by the maintainers of the project, the Operator SDK generated guidelines ([see below](#operator-sdk-generated-guidelines)) might help in making use of the various kustomize configuration files, possibly through the use of `make`.

## Configuration

The operator exposes a few parameters, meant to be set as arguments, though it's possible to use environment variables for some of them. When an environment variable is available, it takes precedence over the flag. 

The parameters are summarized in the table below :

| Flag name                   | Default | Environment variable | Multiple values allowed | Description                                                                                                                                    |
| --------------------------- | ------- | -------------------- | ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `health-probe-bind-address` | `:8081` | -                    | no                      | The address the probe endpoint binds to. Comes from Operator SDK.                                                                              |
| `leader-elect`              | `false` | -                    | no                      | Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager. Comes from Operator SDK. |
| `metrics-bind-address`      | `:8080` | -                    | no                      | The address the metric endpoint binds to. Comes from Operator SDK.                                                                             |  |
| `override-existing-secret`  | false   | -                    | no                      | Update secret linked to s3User if already exist, else noop                                                                                     |
## Minimal rights needed to work

The Operator need at least this rights:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:CreateBucket",
                "s3:GetObject",
                "s3:ListAllMyBuckets",
                "s3:ListBucket",
                "s3:PutObject"
            ],
            "Resource": [
                "arn:aws:s3:::*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "admin:CreatePolicy",
                "admin:GetBucketQuota",
                "admin:GetPolicy",
                "admin:ListPolicy",
                "admin:SetBucketQuota",
                "admin:CreateUser",
                "admin:ListUsers",
                "admin:DeleteUser",
                "admin:GetUser",
                "admin:AddUserToGroup",
                "admin:RemoveUserFromGroup",
                "admin:AttachUserOrGroupPolicy",
                "admin:ListUserPolicies"

            ],
            "Resource": [
                "arn:aws:s3:::*"
            ]
        }
    ]
}

```

## Usage

- The first step is to install the CRDs in your Kubernetes cluster. The Helm chart will do just that, but it is also possible to do it manually - the manifests are in the [`config/crd/bases`](https://github.com/InseeFrLab/s3-operator/tree/main/config/crd/bases) folder.
- With the CRDs available and the operator running, all that's left is to create some custom resources - you'll find some commented examples in the subsections below.
- As soon as a custom resource is created, the operator will react, and create or update a S3 resource accordingly.
- The same will happen if you modify a CR - the operator will adjust the S3 bucket or policy accordingly - with the notable exception that it will not delete paths for buckets.
- Upon deleting a CR, the corresponding bucket or policy will be left as is, as mentioned in the [*Description* section above](#description)

An instance of S3Operator can manage multiple S3. On each resource created you can set where to create it. To add multiple instance of S3 see S3Instance example. On each object deployed you can attach it to an existing s3Instance. If no instance is set on the resource, S3Operator will failback to default instance configured by env var.

### S3Instance example

```yaml
apiVersion: s3.onyxia.sh/v1alpha1
kind: S3Instance
metadata:
  labels:
    app.kubernetes.io/name: bucket
    app.kubernetes.io/instance: bucket-sample
    app.kubernetes.io/part-of: s3-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: s3-operator
  name: s3-default-instance                     # Name of the S3Instance
spec:
  s3Provider: minio                             # Type of the Provider. Can be "mockedS3Provider" or "minio"
  url: https://minio.example.com                # URL of the Provider
  secretRef: minio-credentials                  # Name of the secret containing 2 Keys S3_ACCESS_KEY and S3_SECRET_KEY
  caCertSecretRef: minio-certs                  # Name of the secret containing key ca.crt with cert of s3provider
  region: us-east-1                             # Region of the Provider
  allowedNamespaces: []                         # namespaces allowed to have buckets, policies, ... Wildcard prefix/suffix allowed. If empty only the same namespace as s3instance is allowed
  bucketDeletionEnabled: true                   # Allowed bucket entity suppression on s3instance                             
  policyDeletionEnabled: true                   # Allowed policy entity suppression on s3instance          
  pathDeletionEnabled: true                     # Allowed path entity suppression on s3instance          
  s3UserDeletionEnabled: true                   # Allowed s3User entity suppression on s3instance          
```

### Bucket example

```yaml
apiVersion: s3.onyxia.sh/v1alpha1
kind: Bucket
metadata:
  labels:
    app.kubernetes.io/name: bucket
    app.kubernetes.io/instance: bucket-sample
    app.kubernetes.io/part-of: s3-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: s3-operator
  name: bucket-sample
spec:
  # Bucket name (on S3 server, as opposed to the name of the CR)
  name: dummy-bucket

  # Paths to create on the bucket 
  # As it is not possible to create empty paths on a S3 server, (limitation of either S3,
  # or at least Minio, the only currently implemented provider), this will actually create
  # a .keep file at the deepest folder in the path.
  paths:
    - a_path
    - another/deeper/path
  
  # Quota to set on the bucket, in bytes (so 1000000000 would be 1GB).
  # This is split over two different parameters, although there is only one actual quota
  #   - "default" is required, and is used as the baseline
  #   - "override" is optional, and as the name implies, takes precedence over "default"
  # Though clumsy, this pattern (for lack of a better word) allows to easily change the
  # default quota for every buckets without impacting the ones that might have received
  # a manual change. If this is not useful to you, you can safely skip using "override".
  quota:
    default: 10000000    
    # override: 20000000
  
  # Optionnal, let empty if you have configured the default s3 else use an existing s3Instance
  s3InstanceRef: "s3-default-instance"
  

```

### Policy example

```yaml
apiVersion: s3.onyxia.sh/v1alpha1
kind: Policy
metadata:
  labels:
    app.kubernetes.io/name: policy
    app.kubernetes.io/instance: policy-sample
    app.kubernetes.io/part-of: s3-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: s3-operator
  name: policy-sample
spec:
  # Policy name (on S3 server, as opposed to the name of the CR)
  name: dummy-policy

  # Optionnal, let empty if you have configured the default s3 else use an existing s3Instance
  s3InstanceRef: "s3-default-instance"

  # Content of the policy, as a multiline string 
  # This should be IAM compliant JSON - follow the guidelines of the actual
  # S3 provider you're using, as sometimes only a subset is available.
     The first Statement (Allow ListBucket) should be applied to every user,
  # as s3-operator uses this call to verify that credentials are valid when
  # reconciling an existing user.
  policyContent: >-
    {
      "Version": "2012-10-17",
      "Statement": [
      {
        "Effect": "Allow",
        "Action": [
          "s3:ListBucket"
        ],
        "Resource": [
          "arn:aws:s3:::*"
        ]
      },
      {
        "Effect": "Allow",
        "Action": [
          "s3:*"
        ],
        "Resource": [
          "arn:aws:s3:::dummy-bucket",
          "arn:aws:s3:::dummy-bucket/*"
        ]
      }
      ]
    }
```

### Path example

```yaml
apiVersion: s3.onyxia.sh/v1alpha1
kind: Path
metadata:
  labels:
    app.kubernetes.io/name: path
    app.kubernetes.io/instance: path-sample
    app.kubernetes.io/part-of: s3-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: s3-operator
  name: path-sample
spec:
  # Bucket name (on S3 server, not a Bucket CR's metadata.name)
  bucketName: shared-bucket

  # Paths to create on the bucket 
  paths:
    - /home/alice
    - /home/bob
  
  # Optionnal, let empty if you have configured the default s3 else use an existing s3Instance
  s3InstanceRef: "s3-default-instance"

```

### S3User example

```yaml
apiVersion: s3.onyxia.sh/v1alpha1
kind: S3User
metadata:
  labels:
    app.kubernetes.io/name: user
    app.kubernetes.io/instance: user-sample
    app.kubernetes.io/part-of: s3-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: s3-operator
  name: user-sample
spec:
  accessKey: user-sample
  policies:
    - policy-example1
    - policy-example2
  # Optionnal, let empty if you have configured the default s3 else use an existing s3Instance
  s3InstanceRef: "s3-default-instance"

```

Each S3user is linked to a kubernetes secret which have the same name that the S3User. The secret contains 2 keys: `accessKey` and `secretKey`.

### :info: How works s3InstanceRef

S3InstanceRef can get the following values:
- empty: In this case the s3instance use will be the default one configured at startup if the namespace is in the namespace allowed for this s3Instance
- `s3InstanceName`: In this case the s3Instance use will be the s3Instance with the name `s3InstanceName` in the current namespace (if the current namespace is allowed)
- `namespace/s3InstanceName`: In this case the s3Instance use will be the s3Instance with the name `s3InstanceName` in the namespace `namespace` (if the current namespace is allowed to use this s3Instance)

## Operator SDK generated guidelines

<details>

<summary><strong>Click to fold / unfold</strong></summary>

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

</details>


