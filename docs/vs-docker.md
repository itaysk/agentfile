# Agentfile vs Docker

We love Docker, and think it's been revolutionary to our industry. Agentfile is inspired by some of the concepts that Docker but also intentioally differs on some parts:

## Agentfile vs Dockerfile

Agentfile is fully declarative. Dockerfile is a mix of declarative and imperative commands.

Agentfile describes the artifact's metadata. Dockerfile does not, and relies on build-time input.

## Bundles vs Images

Bundle targets a harness. Image targets a platform (os/arch).

Bundle has no executable binaries. Image has executable binaries.

Bundle is a single portable file. Image is a reference to a manifest which itself is a multi-layer referencial hierachy.

Bundle lives in a filesystem. Image lives in a registry.

Bundle has one file name. Image can have multiple tags referencing the same image.

## af build vs docker build

af doesn't register a bundle implicitly. docker registers an image with the local daemon as soon as it's built.
