const root = document.getElementById("app");
if (root) {
	const title = document.createElement("h1");
	title.textContent = "agent-message-bus";
	root.replaceChildren(title);
}
