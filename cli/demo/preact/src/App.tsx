export function App() {
  return (
    <>
      <div class="h-full text-center font-sans mt8 flex select-none all:transition-40">
        <div class="ma">
          <h2 class="text-5xl fw500">esm.sh</h2>
          <p class="op30 text-lg fw300 m1">
            The <span class="fw600">no-build</span> global content delivery network(CDN).
          </p>
          <div class="m2 flex justify-center text-2xl op30 hover:op80">
            <a
              class="i-carbon-logo-github text-inherit hover:animate-spin"
              href="https://github.com/esm-dev/esm.sh"
              target="_blank"
            />
          </div>
        </div>
      </div>

      <div class="absolute bottom-5 w-full flex justify-center">
        <button class="btn">Click Me</button>
      </div>
    </>
  );
}
