document.addEventListener("click", function(event) {
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  var card = event.target.closest(".video-embed");
  if (!card) return;
  if (card.querySelector("iframe")) return;
  var src = card.dataset.videoEmbed;
  if (!src) return;
  event.preventDefault();

  var frame = document.createElement("iframe");
  frame.src = src;
  frame.title = card.dataset.videoTitle || "";
  frame.allow = "accelerometer; autoplay; encrypted-media; gyroscope; picture-in-picture; web-share";
  frame.allowFullscreen = true;
  frame.referrerPolicy = "strict-origin-when-cross-origin";
  card.replaceChildren(frame);
});
