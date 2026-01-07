## VM Console Access

Virtink exposes a lightweight console endpoint from `virt-daemon` that bridges a WebSocket
to the VM serial socket. This is the fastest way to access the VM console without adding
API subresources.

### Requirements

- A VM with `spec.instance.serial` set (defaults to a Unix socket at `/var/run/virtink/serial.sock`).
- Access to the `virt-daemon` pod (via `kubectl port-forward`).
- A WebSocket client like `websocat`.

### Example VM Spec

```yaml
spec:
  instance:
    kernel:
      image: smartxworks/virtink-kernel-5.15.12
      cmdline: "console=ttyS0 root=/dev/vda rw"
    serial:
      mode: Socket
```

### Connect to Console

1) Port-forward the daemon on the node that hosts the VM:

```bash
kubectl -n virtink-system get vm <vm-name> -o jsonpath='{.status.nodeName}'
kubectl -n virtink-system get pod -l name=virt-daemon -o wide
kubectl -n virtink-system port-forward pod/<virt-daemon-pod> 8082:8082
```

2) Attach using `websocat`:

```bash
websocat ws://127.0.0.1:8082/api/v1/vms/<namespace>/<vm-name>/console
```

### Notes

- If you see `serial socket not found`, ensure the VM is Running and the `serial` mode is `Socket`.
- The console stream is raw; you may want to disable local echo in your terminal.
