"use strict";
// The packaged copy lives beside this loader; development uses the web's source.
const fs = require("node:fs");
const path = require("node:path");
const bundled = path.join(__dirname, "workbench-model.js");
module.exports = require(
  fs.existsSync(bundled)
    ? bundled
    : path.resolve(
        __dirname,
        "../server/cmd/ukinfo-server/web/workbench-model.js",
      ),
);
