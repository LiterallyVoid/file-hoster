document.querySelector('input[type="submit"]').style.display = "none"

const password = document.querySelector("#password")
const private_selfdestruct = document.querySelector("#private-selfdestruct")

const file_selector = document.querySelector("input.files")

const file_drop_target = document.querySelector("#file-drop-target")

function humanBytes(bytes) {
	const levels = [
		"TB",
		"GB",
		"MB",
		"KB",
		"B",
	]

	let level = levels.length - 1

	let fraction = bytes

	while ((fraction / 1024) >= 1 && level > 0) {
		fraction /= 1024
		level--
	}

	return fraction.toFixed(2) + levels[level]
}

async function upload(file) {
	const xhr = new XMLHttpRequest()
	xhr.open("POST", "/new")

	const body = new FormData()
	body.append("password", password.value)
	if (private_selfdestruct.checked) body.append("private-selfdestruct", "on")
	body.append("files", file)

	const element = document.createElement("div")
	element.className = "upload"
	document.body.appendChild(element)

	const flex = document.createElement("div")
	flex.className = "upload__flex"
	element.appendChild(flex)

	const title = document.createElement("a")
	title.className = "upload__title"
	title.textContent = file.name
	flex.appendChild(title)

	const info = document.createElement("span")
	info.className = "upload__info"
	flex.appendChild(info)

	const bar = document.createElement("div")
	bar.className = "upload__bar"
	element.appendChild(bar)

	const url = document.createElement("span")
	url.className = "upload__url"
	element.appendChild(url)

	bar.style.width = "0"

	xhr.upload.addEventListener("progress", e => {
		/* Add one, to avoid division by zero when uploading an empty file. This adjustment is adjustment is so small it should never matter. */
		const uploaded = (e.loaded + 1) / (e.total + 1)

		info.textContent = humanBytes(e.loaded)
		bar.style.width = (uploaded * 100) + "%"
	})

	xhr.addEventListener("error", _ => {
		element.classList.add("upload--failed")
		info.textContent = "failed"
	})

	xhr.addEventListener("load", _ => {
		if (xhr.status != 200) {
			element.classList.add("upload--failed")
			info.textContent = "failed"
			return
		}

		element.classList.add("upload--success")

		const response = JSON.parse(xhr.responseText)

		console.assert(response.ok)
		console.assert(response.uploads.length == 1)

		title.href = response.uploads[0].url

		url.textContent = response.uploads[0].url
		bar.remove()
	})

	console.log("Sending XHRequest")
	xhr.send(body)
}

file_selector.addEventListener("change", _ => {
	if (!file_selector.files.length) return
	for (const file of file_selector.files) {
		upload(file)
	}

	file_selector.value = null
})

// Not sure what this does, but it works now.
file_drop_target.addEventListener("dragenter", event => {
	event.preventDefault()
	event.stopPropagation()
	event.dataTransfer.dropEffect = "copy"
})

file_drop_target.addEventListener("dragover", event => {
	event.preventDefault()
	event.stopPropagation()
	event.dataTransfer.dropEffect = "copy"
})

file_drop_target.addEventListener("drop", event => {
	event.preventDefault()
	event.stopPropagation()

	if (event.dataTransfer == null) return

	for (const file of event.dataTransfer.files) {
		upload(file)
	}
})
