const foo={bar:{}};
const O=Object;
export default ()=>({__proto__:foo}).bar===foo.bar&&!({__proto__:null}instanceof O);
