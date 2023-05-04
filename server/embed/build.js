export async function build(options) {
  if (typeof options === "string") {
    options = { code: options };
  }
  if (!options || !options.code) {
    throw new Error("esm.sh [build] <400> missing code");
  }
  const ret = await fetch("$ORIGIN/build", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options),
  }).then((r) => r.json());
  if (ret.error) {
    throw new Error(
      `esm.sh [build] <${ret.error.status}> ${ret.error.message}`,
    );
  }
  return ret;
}

export default build;
