import hot from "/v135/hot?plugins=tsx,vue,svelte,unocss,md";

hot.unocss.config({
  entry: ["/embed/hot.html"],
});
hot.listen();
