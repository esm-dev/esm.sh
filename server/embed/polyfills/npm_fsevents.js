// https://www.npmjs.com/package/fsevents

export default {
  watch(_dir, _cb) {
    return Promise.resolve();
  },
  getInfo(path, _flags, _id) {
    return {
      event: "mock",
      path,
      type: "file",
      flags: 0x1_00_00_00_00,
      changes: {
        inode: false,
        finder: false,
        access: false,
        xattrs: false,
      },
    };
  },
};
