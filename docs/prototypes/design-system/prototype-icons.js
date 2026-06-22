(function () {
  function createFallbackIcon(name) {
    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "2");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");
    svg.setAttribute("class", "lucide lucide-" + name);
    svg.setAttribute("aria-hidden", "true");

    var title = document.createElementNS("http://www.w3.org/2000/svg", "title");
    title.textContent = name;
    svg.appendChild(title);

    var circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    circle.setAttribute("cx", "12");
    circle.setAttribute("cy", "12");
    circle.setAttribute("r", "8");
    svg.appendChild(circle);

    var path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", "M12 8v8M8 12h8");
    svg.appendChild(path);

    return svg;
  }

  function renderFallbackIcons() {
    document.querySelectorAll("i[data-lucide]").forEach(function (placeholder) {
      var name = placeholder.getAttribute("data-lucide") || "circle";
      placeholder.replaceWith(createFallbackIcon(name));
    });
  }

  function renderPrototypeIcons() {
    if (window.lucide && typeof window.lucide.createIcons === "function") {
      window.lucide.createIcons();
    }

    if (document.querySelector("i[data-lucide]")) {
      renderFallbackIcons();
    }
  }

  window.renderPrototypeIcons = renderPrototypeIcons;
})();
