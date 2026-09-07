// 文章录入页面
document.addEventListener('alpine:init', () => {
    Alpine.data('articleNew', () => ({
        mode: 'manual',
        form: {
            title: '',
            content: '',
            source_url: '',
            category_name: '',
            tags: []
        },
        newTags: '',
        loading: false,
        preview: null,
        previewLoading: false,

        get tagsArray() {
            return this.newTags.split(',').map(t => t.trim()).filter(t => t);
        },

        async submitManual(e) {
            e.preventDefault();
            this.loading = true;

            const res = await fetch('/api/v1/articles', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    ...this.form,
                    source_type: 'manual',
                    tags: this.tagsArray
                })
            });

            this.loading = false;

            if (res.ok) {
                const data = await res.json();
                window.location.href = `/ui/articles/${data.id}`;
            } else {
                const data = await res.json();
                alert(data.error || '提交失败');
            }
        },

        async previewUrl() {
            if (!this.form.source_url) return;

            this.previewLoading = true;
            this.preview = null;

            const res = await fetch('/api/v1/admin/collector/preview', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ url: this.form.source_url })
            });

            this.previewLoading = false;

            if (res.ok) {
                this.preview = await res.json();
            } else {
                const data = await res.json();
                alert(data.error || '采集失败');
            }
        },

        async submitUrl(e) {
            e.preventDefault();
            this.loading = true;

            const res = await fetch('/api/v1/articles', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    ...this.form,
                    source_type: 'url',
                    tags: this.tagsArray
                })
            });

            this.loading = false;

            if (res.ok) {
                const data = await res.json();
                window.location.href = `/ui/articles/${data.id}`;
            } else {
                const data = await res.json();
                alert(data.error || '提交失败');
            }
        },

        usePreview() {
            if (this.preview) {
                this.form.title = this.preview.title || this.form.title;
                this.form.content = this.preview.content || this.form.content;
            }
        }
    }));
});
