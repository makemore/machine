# Hostinger dev VM

This example creates one shared 8GB Hostinger VPS dev machine profile.

Before running, replace the `REPLACE_ME` SSH public key for the `dev` user.

Required environment:

```bash
export HOSTINGER_API_TOKEN="..."
```

Useful optional overrides:

```bash
export HOSTINGER_VPS_ITEM_ID="hostingercom-vps-kvm2-usd-1m"
export HOSTINGER_DATA_CENTER_ID="19"
export HOSTINGER_TEMPLATE_ID="1130"
export HOSTINGER_PAYMENT_METHOD_ID="123456"
```

Run the VM:

```bash
mach -f machine/examples/hostinger-team/Machinefile.dev up
```

By default, `memory: 8gb` maps to Hostinger KVM 2. Set
`HOSTINGER_VPS_ITEM_ID` if your account/catalog uses a different item ID.