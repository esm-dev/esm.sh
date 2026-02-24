import { ReactLogo } from "./components/Logo.tsx";

export function App() {
  return (
    <>
      <div className="center-box absolute opacity-15">
        <ReactLogo />
      </div>
      <div className="center-box relative">
        <h1 className="font-sans text-5xl font-medium text-primary select-none">esm.sh</h1>
        <p className="font-sans text-lg font-normal text-gray-400 text-center">
          The <span className="font-semibold">nobuild</span> cdn for modern web development.
        </p>
        <div className="flex justify-center gap-3 mt-2 text-2xl all:transition-300">
          <a className="logo" href="https://esm.sh" target="_blank" title="Website">
            <img className="size-6" src="/assets/globe.svg" alt="Website" />
          </a>
          <a className="logo" href="https://bsky.app/profile/esm.sh" target="_blank" title="Bluesky">
            <img className="size-6" src="/assets/bluesky.svg" alt="Bluesky" />
          </a>
          <a className="logo" href="https://github.com/esm-dev/esm.sh" target="_blank" title="Github">
            <img className="size-6" src="/assets/github.svg" alt="Github" />
          </a>
        </div>
      </div>
    </>
  );
}
