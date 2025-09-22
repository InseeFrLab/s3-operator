# s3-operator

![Version: 0.9.0](https://img.shields.io/badge/Version-0.9.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.12.0](https://img.shields.io/badge/AppVersion-v0.12.0-informational?style=flat-square)

A Helm chart for deploying an operator to manage S3 resources (eg buckets, policies)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controllerManager.manager.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}` | Set the Container securityContext |
| controllerManager.manager.extraArgs | list | `[]` | Additional Arguments |
| controllerManager.manager.extraEnv | object | `{}` | Additional Environment Variables |
| controllerManager.manager.image.repository | string | `"inseefrlab/s3-operator"` |  |
| controllerManager.manager.image.tag | string | `nil` |  |
| controllerManager.manager.imagePullPolicy | string | `"IfNotPresent"` |  |
| controllerManager.manager.imagePullSecrets | list | `[]` | Configuration for `imagePullSecrets` so that you can use a private images registry. |
| controllerManager.manager.podAnnotations | object | `{}` | Annotations to add to the pod. |
| controllerManager.manager.podLabels | object | `{}` | Labels to add to the pod. |
| controllerManager.manager.podSecurityContext | object | `{"runAsNonRoot":true}` | Set the Pod securityContext |
| controllerManager.manager.priorityClassName | string | `""` | Set the priority class name |
| controllerManager.manager.resources | object | `{"limits":{"cpu":"1000m","memory":"512Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` | Set the resources |
| controllerManager.replicas | int | `1` | Amount of Replicas |
| crds.install | bool | `true` | Install and upgrade CRDs |
| crds.keep | bool | `true` | Keep CRDs on chart uninstall |
| kubernetes.clusterDomain | string | `"cluster.local"` |  |
| kubernetes.overrideExistingSecret | bool | `false` | When creating an S3User, update existing secret with the generated secret key |
| kubernetes.readExistingSecret | bool | `false` | When creating an S3User, read existing secret to retrieve the secret key |
| s3 | object | `{"default":{"accessKey":"accessKey","createNamespace":true,"deletion":{"bucket":true,"path":false,"policy":false,"s3user":false},"enabled":false,"namespace":"s3-operator","region":"us-east-1","s3Provider":"minio","secretKey":"secretKey","url":"https://localhost:9000"}}` | Default S3 Instance |

