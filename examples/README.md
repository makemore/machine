# Machine Examples

This directory contains example Machinefiles demonstrating different use cases.

## Available Examples

### basic-linux
Minimal Linux machine with git and curl.

```bash
cd examples/basic-linux
../../machine up
../../machine ssh
../../machine destroy
```

### python-dev
Python development environment with pip and a cloned repo.

```bash
cd examples/python-dev
../../machine up
../../machine ssh
../../machine destroy
```

### node-dev
Node.js development environment.

```bash
cd examples/node-dev
../../machine up
../../machine ssh
../../machine destroy
```

### full-stack
Complete full-stack environment with Node, Python, PostgreSQL, and Redis.

```bash
cd examples/full-stack
../../machine up
../../machine ssh
../../machine destroy
```

## Testing All Examples

Run the test script to validate all examples:

```bash
./test-examples.sh
```

This will:
1. Build the machine CLI
2. Run `machine up` for each example
3. Verify the machine is running
4. Clean up with `machine destroy`

