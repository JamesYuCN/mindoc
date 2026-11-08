$(function () {

    window.editormdLocales = {
        'zh-CN': {
            placeholder: '本编辑器支持 Markdown 编辑，左边编写，右边预览。',
            contentUnsaved: '编辑内容未保存，需要保存吗？',
            noDocNeedPublish: '没有需要发布的文档',
            loadDocFailed: '文档加载失败',
            fetchDocFailed: '获取当前文档信息失败',
            cannotAddToEmptyNode: '空节点不能添加内容',
            overrideModified: '文档已被其他人修改确定覆盖已存在的文档吗？',
            confirm: '确定',
            cancel: '取消',
            contentsNameEmpty: '目录名称不能为空',
            addDoc: '添加文档',
            edit: '编辑',
            delete: '删除',
            loadFailed: '加载失败请重试',
            tplNameEmpty: '模板名称不能为空',
            tplContentEmpty: '模板内容不能为空',
            saveSucc: '保存成功',
            serverExcept: '服务器异常',
            paramName: '参数名称',
            paramType: '参数类型',
            example: '示例值',
            remark: '备注',
        },
        'en': {
            placeholder: 'This editor supports Markdown editing, writing on the left and previewing on the right.',
            contentUnsaved: 'The edited content is not saved, need to save it?',
            noDocNeedPublish: 'No Document need to be publish',
            loadDocFailed: 'Load Document failed',
            fetchDocFailed: 'Fetch Document info failed',
            cannotAddToEmptyNode: 'Cannot add content to empty node',
            overrideModified: 'The document has been modified by someone else, are you sure to overwrite the document?',
            confirm: 'Confirm',
            cancel: 'Cancel',
            contentsNameEmpty: 'Document Name cannot be empty',
            addDoc: 'Add Document',
            edit: 'Edit',
            delete: 'Delete',
            loadFailed: 'Failed to load, please try again',
            tplNameEmpty: 'Template name cannot be empty',
            tplContentEmpty: 'Template content cannot be empty',
            saveSucc: 'Save success',
            serverExcept: 'Server Exception',
            paramName: 'Parameter',
            paramType: 'Type',
            example: 'Example',
            remark: 'Remark',
        }
    };

    /**
     * 点第一次时，把选中的文字变成同时加粗和斜体
     * 保持光标选区不变，点第二次时，把加粗斜体的文字变成普通文本
     */
    var customMenuA = Cherry.createMenuHook('加粗斜体', {
        iconName: 'font',
        onClick: function (selection) {
            // 获取用户选中的文字，调用getSelection方法后，如果用户没有选中任何文字，会尝试获取光标所在位置的单词或句子
            let $selection = this.getSelection(selection) || '同时加粗斜体';
            // 如果是单选，并且选中内容的开始结束内没有加粗语法，则扩大选中范围
            if (!this.isSelections && !/^\s*(\*\*\*)[\s\S]+(\1)/.test($selection)) {
                this.getMoreSelection('***', '***', () => {
                    const newSelection = this.editor.getSelection();
                    const isBoldItalic = /^\s*(\*\*\*)[\s\S]+(\1)/.test(newSelection);
                    if (isBoldItalic) {
                        $selection = newSelection;
                    }
                    return isBoldItalic;
                });
            }
            // 如果选中的文本中已经有加粗语法了，则去掉加粗语法
            if (/^\s*(\*\*\*)[\s\S]+(\1)/.test($selection)) {
                return $selection.replace(/(^)(\s*)(\*\*\*)([^\n]+)(\3)(\s*)($)/gm, '$1$4$7');
            }
            /**
             * 注册缩小选区的规则
             *    注册后，插入“***TEXT***”，选中状态会变成“***【TEXT】***”
             *    如果不注册，插入后效果为：“【***TEXT***】”
             */
            this.registerAfterClickCb(() => {
                this.setLessSelection('***', '***');
            });
            return $selection.replace(/(^)([^\n]+)($)/gm, '$1***$2***$3');
        }
    });
    /**
     * 定义一个空壳，用于自行规划cherry已有工具栏的层级结构
     */
    var customMenuB = Cherry.createMenuHook('发布', {
        iconName: 'publish',
        onClick: releaseDocument,
    });

    var customMenuC = Cherry.createMenuHook("返回", {
        iconName: 'back',
        onClick: backWard,
    })

    var customMenuD = Cherry.createMenuHook('保存', {
        iconName: 'save',
        onClick: saveDocument,
        // 新版 createMenuHook 不再支持 id 选项，通过 afterInit 给按钮补充 id，供未保存状态判断使用
        afterInit: function (btnDom) {
            btnDom.id = 'markdown-save';
            btnDom.classList.add('disabled');
        }
    });

    var customMenuE = Cherry.createMenuHook('边栏', {
        iconName: 'sider',
        onClick: siderChange,
    });

    var customMenuF = Cherry.createMenuHook('历史', {
        iconName: 'history',
        onClick: showHistory,
    });

    /**
     * 附件按钮：选择本地文件上传后，在光标处插入 [文件名](url) 链接
     * 上传复用 myFileUpload（window.fileUploadURL，字段 editormd-file-file），交互参考 editor.md 的 file-dialog 实现
     */
    var customMenuG = Cherry.createMenuHook('附件', {
        icon: {
            type: 'svg',
            content: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" width="16" height="16" fill="currentColor"><path d="M4.5 3a2.5 2.5 0 0 1 5 0v9a1.5 1.5 0 0 1-3 0V5a.5.5 0 0 1 1 0v7a.5.5 0 0 0 1 0V3a1.5 1.5 0 1 0-3 0v9a2.5 2.5 0 0 0 5 0V5a.5.5 0 0 1 1 0v7a3.5 3.5 0 1 1-7 0V3z"/></svg>',
            iconStyle: 'width:16px;height:16px;vertical-align:-2px;',
        },
        onClick: function () {
            uploadAttachments();
        },
    });

    let customMenuTools =  Cherry.createMenuHook('工具',  {
        iconName: '',
        subMenuConfig: [
            {
                iconName: 'word',
                name: 'Word转笔记',
                onclick: ()=>{
                    let converter = new WordToHtmlConverter();
                    converter.handleFileSelect(function (response) {
                        if (response.messages.length) {
                            let messages = response.messages.map((item)=>{
                                return item.message + "<br/>";
                            }).join('\n');
                            layer.msg(messages);
                        }
                        converter.replaceHtmlBase64(response.value).then((html)=>{
                            window.editor.insertValue(html);
                        });
                    })
                }
            },
            {
                noIcon: true,
                name: 'Htm转Markdown',
                onclick: ()=>{
                    let converter = new HtmlToMarkdownConverter();
                    converter.handleFileSelect(function (response) {
                        window.editor.insertValue(response);
                    })
                }
            }
        ]
    });


    var basicConfig = {
        id: 'manualEditorContainer',
        externals: {
            echarts: window.echarts,
            katex: window.katex,
            MathJax: window.MathJax,
        },
        isPreviewOnly: false,
        fileUpload: myFileUpload,
        engine: {
            global: {
                urlProcessor(url, srcType) {
                    //console.log(`url-processor`, url, srcType);
                    return url;
                },
            },
            syntax: {
                codeBlock: {
                    theme: 'twilight',
                },
                table: {
                    enableChart: false,
                    // chartEngine: Engine Class
                },
                fontEmphasis: {
                    allowWhitespace: false, // 是否允许首尾空格
                },
                strikethrough: {
                    needWhitespace: false, // 是否必须有前后空格
                },
                mathBlock: {
                    // 使用本地 katex（模板已加载 /static/katex/katex.min.js + css）：
                    // 同步渲染、无 CDN 依赖，且支持 \Huge 等字号命令与 \color/\textcolor 颜色命令（MathJax 不支持字号命令）
                    engine: 'katex',
                },
                inlineMath: {
                    engine: 'katex',
                },
                emoji: {
                    useUnicode: false,
                    customResourceURL: 'https://github.githubassets.com/images/icons/emoji/unicode/${code}.png?v8',
                    upperCase: true,
                },
                // toc: {
                //     tocStyle: 'nested'
                // }
                // 'header': {
                //   strict: false
                // }
            },
        },
        toolbars: {
            toolbar: [
                'customMenuCName',
                'customMenuDName',
                'customMenuBName',
                'customMenuEName',
                'undo',
                'redo',
                'bold',
                'italic',
                {
                    strikethrough: ['strikethrough', 'underline', 'sub', 'sup', 'ruby', 'customMenuAName'],
                },
                'size',
                '|',
                'color',
                'header',
                '|',
                'drawIo',
                '|',
                'ol',
                'ul',
                'checklist',
                'panel',
                'detail',
                '|',
                'formula',
                {
                    insert: ['image', 'audio', 'video', 'link', 'hr', 'br', 'code', 'formula', 'toc', 'table', 'pdf', 'word', 'ruby'],
                },
                'customMenuGName',
                'graph',
                'togglePreview',
                'settings',
                'switchModel',
                'export',
                'customMenuFName',
                'customMenuToolsName'
            ],
            bubble: ['bold', 'italic', 'underline', 'strikethrough', 'sub', 'sup', 'quote', 'ruby', '|', 'size', 'color'], // array or false
            sidebar: ['mobilePreview', 'copy', 'codeTheme', 'theme'],
            customMenu: {
                customMenuAName: customMenuA,
                customMenuBName: customMenuB,
                customMenuCName: customMenuC,
                customMenuDName: customMenuD,
                customMenuEName: customMenuE,
                customMenuFName: customMenuF,
                customMenuGName: customMenuG,
                customMenuToolsName: customMenuTools,
            },
        },
        drawioIframeUrl: '/static/cherry/drawio_demo.html',
        editor: {
            defaultModel: 'edit&preview',
            height: "100%",
        },
        previewer: {
            // 自定义markdown预览区域class
            // className: 'markdown'
        },
        keydown: [],
        //extensions: [],
        //callback: {
        //changeString2Pinyin: pinyin,
        //}
    };

    fetch('').then((response) => response.text()).then((value) => {
        //var markdownarea = document.getElementById("markdown_area").value
        var config = Object.assign({}, basicConfig);// { value: markdownarea });// { value: value });不显示获取的初始化值
        window.editor = new Cherry(config);
        // 新版 cherry 使用 CodeMirror6，getCodeMirror() 返回原生 EditorView（无 on 方法），
        // CM6Adapter（window.editor.editor.editor）提供 CM5 兼容的 change 事件：handler(adapter, changeObj)
        window.editor.editor.editor.on('change', (e, detail)=>{
            resetEditorChanged(true);
        });
        openLastSelectedNode();
    });

    /***
     * 加载指定的文档到编辑器中
     * @param $node
     */
    window.loadDocument = function ($node) {
        var index = layer.load(1, {
            shade: [0.1, '#fff'] // 0.1 透明度的白色背景
        });

        $.get(window.editURL + $node.node.id).done(function (res) {
            layer.close(index);

            if (res.errcode === 0) {
                window.isLoad = true;
                try {
                    window.editor.setTheme(res.data.markdown_theme);
                    window.editor.setMarkdown(res.data.markdown);
                } catch (e) {
                    console.log(e);
                }
                var node = { "id": res.data.doc_id, 'parent': res.data.parent_id === 0 ? '#' : res.data.parent_id, "text": res.data.doc_name, "identify": res.data.identify, "version": res.data.version };
                pushDocumentCategory(node);
                window.selectNode = node;
                // 更新浏览器地址栏和预览链接
                var docIdentify = res.data.identify;
                if (docIdentify && window.history) {
                    var editBase = window.editURL.replace(/\/content\/$/, '/edit/');
                    var newURL = editBase + docIdentify;
                    if (window._initialDocLoaded) {
                        history.pushState({ docId: res.data.doc_id }, res.data.doc_name, newURL);
                    } else {
                        history.replaceState({ docId: res.data.doc_id }, res.data.doc_name, newURL);
                        window._initialDocLoaded = true;
                    }
                    var readBase = window.editURL.replace(/\/api\/([^/]+)\/content\/$/, '/docs/$1/');
                    $('a:has(i[name="preview-open"])').attr('href', readBase + docIdentify);
                }
                pushVueLists(res.data.attach);
                setLastSelectNode($node);
            } else {
                layer.msg(editormdLocales[lang].loadDocFailed);
            }
        }).fail(function () {
            layer.close(index);
            layer.msg(editormdLocales[lang].loadDocFailed);
        });
    };

    /**
     * 保存文档到服务器
     * @param $is_cover 是否强制覆盖
     */
    function saveDocument($is_cover, callback) {
        var index = null;
        var node = window.selectNode;
        var content = window.editor.getMarkdown();
        var html = window.editor.getHtml(true);
        var markdownTheme = getCherryTheme();
        var version = "";

        if (!node) {
            layer.msg(editormdLocales[lang].fetchDocFailed);
            return;
        }
        if (node.a_attr && node.a_attr.disabled) {
            layer.msg(editormdLocales[lang].cannotAddToEmptyNode);
            return;
        }

        var doc_id = parseInt(node.id);

        for (var i in window.documentCategory) {
            var item = window.documentCategory[i];

            if (item.id === doc_id) {
                version = item.version;
                break;
            }
        }
        $.ajax({
            beforeSend: function () {
                index = layer.load(1, { shade: [0.1, '#fff'] });
                window.saveing = true;
            },
            url: window.editURL,
            data: { "identify": window.book.identify, "doc_id": doc_id, "markdown": content, "html": html, "markdown_theme": markdownTheme, "cover": $is_cover ? "yes" : "no", "version": version },
            type: "post",
            timeout: 30000,
            dataType: "json",
            success: function (res) {
                if (res.errcode === 0) {
                    resetEditorChanged(false);
                    for (var i in window.documentCategory) {
                        var item = window.documentCategory[i];

                        if (item.id === doc_id) {
                            window.documentCategory[i].version = res.data.version;
                            break;
                        }
                    }
                    $.each(window.documentCategory, function (i, item) {
                        var $item = window.documentCategory[i];

                        if (item.id === doc_id) {
                            window.documentCategory[i].version = res.data.version;
                        }
                    });
                    if (typeof callback === "function") {
                        callback();
                    }

                } else if (res.errcode === 6005) {
                    var confirmIndex = layer.confirm(editormdLocales[lang].overrideModified, {
                        btn: [editormdLocales[lang].confirm, editormdLocales[lang].cancel] // 按钮
                    }, function () {
                        layer.close(confirmIndex);
                        saveDocument(true, callback);
                    });
                } else {
                    layer.msg(res.message);
                }
            },
            error: function (XMLHttpRequest, textStatus, errorThrown) {
                layer.msg(window.editormdLocales[window.lang].serverExcept + errorThrown);
            },
            complete: function () {
                layer.close(index);
                window.saveing = false;
            }
        });
    }


    /**
     * 设置编辑器变更状态
     * @param $is_change
     */
    function resetEditorChanged($is_change) {
        if ($is_change && !window.isLoad) {
            $("#markdown-save").removeClass('disabled').addClass('change');
        } else {
            $("#markdown-save").removeClass('change').addClass('disabled');
        }
        window.isLoad = false;
    }

    /**
     * 返回上一个页面
     */
    function backWard() {
        if (document.referrer == "") { // 没有上一级
            var homepage = window.location.origin;
            window.location.href = homepage; // 返回首页
            return;
        }
        window.location.href = document.referrer;
    }

    /**
     * 发布文档
     */

    function releaseDocument() {
        if (Object.prototype.toString.call(window.documentCategory) === '[object Array]' && window.documentCategory.length > 0) {
            if ($("#markdown-save").hasClass('change')) {
                var confirm_result = confirm(editormdLocales[lang].contentUnsaved);
                if (confirm_result) {
                    saveDocument(false, releaseBook);
                    return;
                }
            }
            releaseBook();
            return
        }
        layer.msg(editormdLocales[lang].noDocNeedPublish)
    }


    /**
     * 显示/隐藏边栏
     */

    function siderChange() {
        $("#manualCategory").toggle(0, "swing", function () {
            var $then = $("#manualEditorContainer");
            var left = parseInt($then.css("left"));
            if (left > 0) {
                window.editorContainerLeft = left;
                $then.css("left", "0");
            } else {
                $then.css("left", window.editorContainerLeft + "px");
            }
        });
    }

    /**
     * 显示文档历史
     */

    function showHistory() {
        window.documentHistory();
    }

    /**
     * 添加文档
     */
    $("#addDocumentForm").ajaxForm({
        beforeSubmit: function () {
            var doc_name = $.trim($("#documentName").val());
            if (doc_name === "") {
                return showError(editormdLocales[lang].contentsNameEmpty, "#add-error-message")
            }
            $("#btnSaveDocument").button("loading");
            return true;
        },
        success: function (res) {
            if (res.errcode === 0) {
                var data = {
                    "id": res.data.doc_id,
                    'parent': res.data.parent_id === 0 ? '#' : res.data.parent_id,
                    "text": res.data.doc_name,
                    "identify": res.data.identify,
                    "version": res.data.version,
                    state: { opened: res.data.is_open == 1 },
                    a_attr: { is_open: res.data.is_open == 1, href: window.editURL.replace(/\/content\/$/, '/edit/') + res.data.identify }
                };

                var node = window.treeCatalog.get_node(data.id);
                if (node) {
                    window.treeCatalog.rename_node({ "id": data.id }, data.text);
                    $("#sidebar").jstree(true).get_node(data.id).a_attr.is_open = data.state.opened;
                } else {
                    window.treeCatalog.create_node(data.parent, data);
                    window.treeCatalog.deselect_all();
                    window.treeCatalog.select_node(data);
                }
                pushDocumentCategory(data);
                $("#markdown-save").removeClass('change').addClass('disabled');
                $("#addDocumentModal").modal('hide');
            } else {
                showError(res.message, "#add-error-message");
            }
            $("#btnSaveDocument").button("reset");
        }
    });

    /**
     * 文档目录树
     */
    // 为目录树节点注入 href，使鼠标悬浮时浏览器状态栏显示对应编辑 URL
    (function () {
        var editBase = window.editURL.replace(/\/content\/$/, '/edit/');
        window.documentCategory.forEach(function (item) {
            if (item.identify && !(item.a_attr && item.a_attr.disabled)) {
                if (!item.a_attr) item.a_attr = {};
                item.a_attr.href = editBase + item.identify;
            }
        });
    }());
    $("#sidebar").jstree({
        'plugins': ["wholerow", "types", 'dnd', 'contextmenu'],
        "types": {
            "default": {
                "icon": false  // 删除默认图标
            }
        },
        'core': {
            'worker':true,
            'check_callback': true,
            "multiple": false,
            'animation': 0,
            "data": window.documentCategory
        },
        "contextmenu": {
            show_at_node: false,
            select_node: false,
            "items": {
                "添加文档": {
                    "separator_before": false,
                    "separator_after": true,
                    "_disabled": false,
                    "label": window.editormdLocales[window.lang].addDoc,//"添加文档",
                    "icon": "fa fa-plus",
                    "action": function (data) {
                        var inst = $.jstree.reference(data.reference),
                            node = inst.get_node(data.reference);

                        openCreateCatalogDialog(node);
                    }
                },
                "编辑": {
                    "separator_before": false,
                    "separator_after": true,
                    "_disabled": false,
                    "label": window.editormdLocales[window.lang].edit,
                    "icon": "fa fa-edit",
                    "action": function (data) {
                        var inst = $.jstree.reference(data.reference);
                        var node = inst.get_node(data.reference);
                        openEditCatalogDialog(node);
                    }
                },
                "删除": {
                    "separator_before": false,
                    "separator_after": true,
                    "_disabled": false,
                    "label": window.editormdLocales[window.lang].delete,
                    "icon": "fa fa-trash-o",
                    "action": function (data) {
                        var inst = $.jstree.reference(data.reference);
                        var node = inst.get_node(data.reference);
                        openDeleteDocumentDialog(node);
                    }
                }
            }
        }
    }).on("ready.jstree", function () {
        window.treeCatalog = $("#sidebar").jstree(true);

        //如果没有选中节点则选中默认节点
        // openLastSelectedNode();
    }).on('select_node.jstree', function (node, selected) {

        if ($("#markdown-save").hasClass('change')) {
            if (confirm(window.editormdLocales[window.lang].contentUnsaved)) {
                saveDocument(false, function () {
                    loadDocument(selected);
                });
                return true;
            }
        }
        //如果是空目录则直接出发展开下一级功能
        if (selected.node.a_attr && selected.node.a_attr.disabled) {
            selected.instance.toggle_node(selected.node);
            return false
        }


        loadDocument(selected);
    }).on("move_node.jstree", jstree_save).on("delete_node.jstree", function ($node, $parent) {
        openLastSelectedNode();
    });
    /**
     * 打开文档模板
     */
    $("#documentTemplateModal").on("click", ".section>a[data-type]", function () {
        var $this = $(this).attr("data-type");
        if ($this === "customs") {
            $("#displayCustomsTemplateModal").modal("show");
            return;
        }
        var body = $("#template-" + $this).html();
        if (body) {
            window.isLoad = true;
            // 新版 cherry 实例不再提供 clear()/setCursor()，改用 setMarkdown 和 Editor 模块的 setCursor
            window.editor.setMarkdown('');
            window.editor.insertValue(body);
            window.editor.editor.setCursor(0, 0);
            resetEditorChanged(true);
        }
        $("#documentTemplateModal").modal('hide');
    });

    document.addEventListener('keydown', function(event) {
        if (event.ctrlKey && event.key === 's') {
            event.preventDefault();
            saveDocument(true, null);
        }
    });
});

/**
 * 附件上传：打开编辑页自带的"上传附件"弹窗（与 editor.md 完全相同的交互）。
 * 弹窗由模板 #uploadAttachModal + WebUploader 实现（拖拽/点击上传、进度条、Vue 附件列表），
 * 上传的附件绑定到当前文档（doc_id），成功后在弹窗内显示可点击的文件链接。
 */
function uploadAttachments() {
    $("#uploadAttachModal").modal("show");
}

/**
 * 获取当前 markdown 主题（新版 cherry 移除了实例方法 getTheme）。
 * 优先从容器 class 读取，其次读 localStorage，返回形如 theme__default 的完整类名，
 * 与旧版 getTheme() 返回值保持一致。
 */
function getCherryTheme() {
    var theme = 'default';
    try {
        if (window.editor && window.editor.wrapperDom) {
            var match = /theme__([^\s]+)/.exec(window.editor.wrapperDom.className || '');
            if (match) {
                theme = match[1];
            }
        }
        if (theme === 'default') {
            var localTheme = localStorage.getItem('cherry-theme');
            if (localTheme) {
                theme = localTheme;
            }
        }
    } catch (e) {
        console.log(e);
    }
    return 'theme__' + theme;
}

function myFileUpload(file, callback) {
    // 创建 FormData 对象以便包含要上传的文件
    var formData = new FormData();
    formData.append("editormd-file-file", file); // "file" 是与你的服务端接口相对应的字段名
    var layerIndex = 0;
    // AJAX 请求
    $.ajax({
        url: window.fileUploadURL, // 确保此 URL 是文件上传 API 的正确 URL
        type: "POST",
        async: false, // 3xxx 20240609这里修改为同步，保证cherry批量上传图片时，插入的图片名称是正确的，否则，插入的图片名称都是最后一个名称
        dataType: "json",
        data: formData,
        processData: false, // 必须设置为 false，因为数据是 FormData 对象，不需要对数据进行序列化处理
        contentType: false, // 必须设置为 false，因为是 FormData 对象，jQuery 将不会设置内容类型头
        
        beforeSend: function () {
            layerIndex = layer.load(1, {
                shade: [0.1, '#fff'] // 0.1 透明度的白色背景
            });
        },
        
        error: function () {
            layer.close(layerIndex);
            layer.msg(locales[lang].uploadFailed);
        },
        success: function (data) {
            layer.close(layerIndex);
            // 验证data是否为数组
            if (data.errcode !== 0) {
                layer.msg(data.message);
            } else {
                callback(data.url); // 假设返回的 JSON 中包含上传文件的 URL，调用回调函数并传入 URL
            }
        }
    });
}