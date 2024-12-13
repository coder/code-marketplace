
import { verify } from "@vscode/vsce-sign";


if(process.argv.length < 2) {
    console.log("Usage: node main.js <extension.vsix> <extension.sigzip>")
    process.exit(1)
}

verify(process.argv[0], process.argv[1], "true").then((x) => {
    console.log(x)
})
