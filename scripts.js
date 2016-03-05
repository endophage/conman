$(function() {
	template = $('#app-template').html();
	Mustache.parse(template);
	$.get("http://104.131.156.213:8080", function(data, stat, xhr){
		$.each(data, function(idx, entry){
			rendered = Mustache.render(template, {name:entry.Name, appname:entry.Name, imgurl: entry.URL});
			$("#apps").append(rendered);
		})
	});

	$("#apps").on("click", "a.install", function(e) {
		$(this).removeClass("btn-primary")
		$(this).attr("disabled", "disabled")
		$(this).text("Installed!")
	});
})
